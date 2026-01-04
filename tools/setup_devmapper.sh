#!/bin/sh
set -eu

POOL_NAME="${1:-fc-dev-pool2}"
DATA_DIR="/var/lib/containerd/devmapper2"
DATA_SIZE="10G"
META_SIZE="2G"

sudo mkdir -p "$DATA_DIR"
DATA_FILE="$DATA_DIR/data-device"
META_FILE="$DATA_DIR/meta-device"

if [ ! -f "$DATA_FILE" ]; then
    sudo truncate -s "$DATA_SIZE" "$DATA_FILE"
fi
if [ ! -f "$META_FILE" ]; then
    sudo truncate -s "$META_SIZE" "$META_FILE"
fi

# Setup loop devices
DATA_DEV=$(sudo losetup --find --show "$DATA_FILE")
META_DEV=$(sudo losetup --find --show "$META_FILE")

echo "Data: $DATA_DEV"
echo "Meta: $META_DEV"

SECTOR_SIZE=512
DATA_SIZE_BYTES=$(sudo blockdev --getsize64 -q "$DATA_DEV")
LENGTH_SECTORS=$((DATA_SIZE_BYTES / SECTOR_SIZE))
DATA_BLOCK_SIZE=128
LOW_WATER_MARK=32768

TABLE="0 ${LENGTH_SECTORS} thin-pool ${META_DEV} ${DATA_DEV} ${DATA_BLOCK_SIZE} ${LOW_WATER_MARK} 1 skip_block_zeroing"

echo "Creating pool $POOL_NAME..."
echo "$TABLE" | sudo dmsetup create "$POOL_NAME"
sudo dmsetup mknodes "$POOL_NAME"
echo "Done."
