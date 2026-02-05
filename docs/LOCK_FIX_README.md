# Bitmain ASIC Device Lock Resolution

## Problem
The `bitmain_asic` kernel module has a reference count of 1, preventing hasher-server from accessing `/dev/bitmain-asic`.

## Quick Fix

### Option 1: Automated Deployment (Recommended)
```bash
./scripts/deploy-fix.sh
```

This will:
1. Deploy diagnostic and fix scripts to the Antminer
2. Run diagnostics to identify the lock holder
3. Attempt to free the device
4. Show results

### Option 2: Manual Commands

SSH to the device:
```bash
ssh -o KexAlgorithms=+diffie-hellman-group14-sha1 \
    -o HostKeyAlgorithms=+ssh-rsa \
    -o StrictHostKeyChecking=no \
    root@192.168.12.151
```

Run diagnostics:
```bash
# Check module status
lsmod | grep bitmain_asic

# Find processes using the device
for pid in $(ls /proc | grep -E '^[0-9]+$'); do
    if [ -d "/proc/$pid/fd" ]; then
        for fd in /proc/$pid/fd/*; do
            if [ -L "$fd" ]; then
                link=$(readlink "$fd" 2>/dev/null)
                if echo "$link" | grep -q "bitmain"; then
                    echo "PID $pid: $(cat /proc/$pid/cmdline 2>/dev/null | tr '\0' ' ')"
                fi
            fi
        done
    fi
done
```

Kill CGMiner and related processes:
```bash
killall -9 cgminer 2>/dev/null
sleep 2
```

Attempt module unload:
```bash
rmmod bitmain_asic
```

If that fails, try force unload:
```bash
rmmod -f bitmain_asic
```

### Option 3: Reboot (Nuclear Option)
If nothing else works:
```bash
reboot -f
```

After reboot, the module should be unloaded and you can:
1. Start hasher-server directly (it may load the module itself)
2. Or load the module manually: `modprobe bitmain_asic`

## What Each Script Does

### diagnose_lock.sh
- Checks module status and reference count
- Identifies processes using the module
- Lists open file descriptors on the device
- Shows CGMiner processes
- Displays kernel messages

### fix_lock.sh
- Kills all CGMiner processes
- Kills processes with device open
- Attempts normal module unload
- Attempts forced module unload
- Verifies results

### nuclear_unload.sh
- Force kills all user processes
- Attempts aggressive module removal
- Last resort before reboot

## Expected Outcome

After running the fix:
- `lsmod | grep bitmain_asic` should return nothing (module unloaded)
- `/dev/bitmain-asic` may be removed (will be recreated when module reloads)
- hasher-server can then access the device

## Starting hasher-server

Once the device is free:
```bash
./hasher-server -port 8888
```

Verify it's running:
```bash
netstat -tlnp | grep 8888
```

## Troubleshooting

If the module reference count is held by a kernel thread (not a process):
1. Check `ps | grep '\['` to see kernel threads
2. The only solution may be reboot
3. The reference might be from a previous cgminer crash

If you get "operation not permitted" even after unloading:
- Check SELinux/AppArmor (unlikely on OpenWrt)
- Verify you're running as root
- Check device permissions: `ls -la /dev/bitmain-asic`
