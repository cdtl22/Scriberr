#!/bin/bash
set -e

# Default values
PUID=${PUID:-1000}
PGID=${PGID:-1000}

echo "=== Scriberr Container Setup ==="
echo "Requested UID: $PUID, GID: $PGID"
# export LD_LIBRARY_PATH=$LD_LIBRARY_PATH:/usr/lib/x86_64-linux-gnu/
# echo "LD_LIBRARY_PATH is: $LD_LIBRARY_PATH"

# chown only when this path is not already owned by PUID:PGID.
# Pass "recursive" as the second argument to chown -R (use on volume roots sparingly).
ensure_ownership() {
    local path="$1"
    local recursive="${2:-}"

    [ -e "$path" ] || return 0

    local uid gid
    uid=$(stat -c '%u' "$path")
    gid=$(stat -c '%g' "$path")
    if [ "$uid" = "$PUID" ] && [ "$gid" = "$PGID" ]; then
        return 0
    fi

    if [ "$recursive" = "recursive" ]; then
        chown -R "$PUID:$PGID" "$path"
    else
        chown "$PUID:$PGID" "$path"
    fi
}

# Function to setup user if needed
setup_user() {
    local target_uid=$1
    local target_gid=$2

    # Check if we need to modify the user
    if [ "$target_uid" != "1000" ] || [ "$target_gid" != "1000" ]; then
        echo "Setting up custom user with UID=$target_uid, GID=$target_gid..."

        # Check if group already exists with different GID
        if getent group "$target_gid" >/dev/null 2>&1; then
            echo "Group with GID $target_gid already exists, using it"
        else
            # Modify existing group or create new one
            groupmod -g "$target_gid" appuser 2>/dev/null || {
                groupadd -g "$target_gid" appgroup
                usermod -g "$target_gid" appuser
            }
        fi

        # Modify user UID
        usermod -u "$target_uid" appuser 2>/dev/null || {
            echo "Warning: Could not change user ID, continuing with existing user"
        }

    else
        echo "Using default user (UID=1000, GID=1000)"
    fi
}

# Setup the user (only if running as root)
if [ "$(id -u)" = "0" ]; then
    setup_user "$PUID" "$PGID"

    # Set up directories with proper ownership
    echo "Setting up data directories..."
    mkdir -p /app/data/uploads /app/data/transcripts /app/whisperx-env

    # Files baked into the image (usermod does not reassign these from UID 1000).
    ensure_ownership /app/scriberr
    if [ -d /app/bin ]; then
        ensure_ownership /app/bin recursive
    fi

    # Persistent volumes: full recursive chown only when the mount root is wrong.
    ensure_ownership /app/data recursive
    ensure_ownership /app/whisperx-env recursive

    # mkdir -p may create these as root while the volume root already matches PUID:PGID.
    ensure_ownership /app/data/uploads
    ensure_ownership /app/data/transcripts

    echo "=== Setup Complete ==="
    echo "Switching to user appuser (UID=$PUID, GID=$PGID) and starting application..."

    # Switch to the appuser and execute the command
    exec gosu appuser "$@"
else
    echo "Running as non-root user UID=$(id -u), GID=$(id -g)"

    # Just ensure directories exist
    mkdir -p /app/data/uploads /app/data/transcripts /app/whisperx-env 2>/dev/null || true

    echo "=== Setup Complete ==="
    echo "Starting Scriberr application..."

    # Execute directly
    exec "$@"
fi
