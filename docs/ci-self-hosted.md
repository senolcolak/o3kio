# Self-Hosted CI Runner Provisioning

The `libvirt-smoke` workflow runs on a self-hosted GitHub Actions runner with
`/dev/kvm` access. GitHub-hosted runners cannot boot real VMs because nested
KVM is not exposed in their VM images. This guide covers what the runner host
needs and how to register it.

## Why self-hosted

The libvirt smoke test boots an actual KVM domain via Nova in real mode and
asserts that `virsh list` shows the domain after server create and that it is
gone after delete. That requires:

- A writable `/dev/kvm` for the runner user
- `libvirtd` running on `qemu:///system`
- `qemu-system-x86_64`

None of those are available on GitHub-hosted runners. Hetzner, OVH, or a spare
Intel/AMD box at home all work. ARM hosts work too but need the `aarch64` qemu
package and a different VM definition — out of scope here.

## Host prerequisites

Tested on Ubuntu 22.04 LTS and 24.04 LTS. Should work on any modern Linux with
KVM, but commands below assume Debian/Ubuntu.

### CPU virtualization

```bash
# Must print at least 1
egrep -c '(vmx|svm)' /proc/cpuinfo

# Should print "KVM acceleration can be used"
sudo apt-get install -y cpu-checker
sudo kvm-ok
```

If the host is itself a VM, enable nested virtualization in the hypervisor
first. On Proxmox: set CPU type to `host` and enable nested. On VMware: tick
"Expose hardware-assisted virtualization to the guest OS".

### Packages

```bash
sudo apt-get update
sudo apt-get install -y \
    qemu-system-x86 \
    qemu-utils \
    libvirt-daemon-system \
    libvirt-clients \
    bridge-utils \
    curl \
    jq
```

### Runner user permissions

Create a dedicated user for the runner. The runner user needs to be in the
`kvm` and `libvirt` groups so it can talk to `qemu:///system` without sudo for
the read-only checks, and have passwordless sudo for the test script itself
(which needs root to write to libvirt's system socket).

```bash
sudo useradd -m -s /bin/bash ghrunner
sudo usermod -aG kvm,libvirt ghrunner

# Passwordless sudo for the smoke test only.
sudo tee /etc/sudoers.d/ghrunner-libvirt <<'EOF'
ghrunner ALL=(root) NOPASSWD: /home/ghrunner/actions-runner/_work/o3kio/o3kio/test/libvirt_smoke_test.sh
EOF
sudo chmod 440 /etc/sudoers.d/ghrunner-libvirt
```

Verify:

```bash
sudo -u ghrunner -- bash -c 'test -w /dev/kvm && echo OK'
sudo -u ghrunner -- virsh -c qemu:///system list
```

Both must succeed before registering the runner.

## Register the GitHub runner

In the GitHub UI: **Settings → Actions → Runners → New self-hosted runner**.
Pick Linux x64. Copy the registration token (it expires in an hour).

```bash
sudo -iu ghrunner
mkdir -p ~/actions-runner && cd ~/actions-runner
curl -o actions-runner-linux-x64.tar.gz -L \
    https://github.com/actions/runner/releases/latest/download/actions-runner-linux-x64-2.319.1.tar.gz
tar xzf actions-runner-linux-x64.tar.gz

# Use the token from the GitHub UI. Add the `kvm` label so workflows that
# request runs-on: [self-hosted, kvm] match this runner.
./config.sh --url https://github.com/<owner>/<repo> --token <TOKEN> --labels kvm
```

Install as a systemd service so it survives reboots:

```bash
sudo ./svc.sh install ghrunner
sudo ./svc.sh start
sudo ./svc.sh status
```

The runner should now appear as **Idle** in the GitHub Settings page with
labels `self-hosted`, `Linux`, `X64`, `kvm`.

## Verifying end-to-end

Trigger the workflow manually from the Actions tab (`workflow_dispatch`) or
push a change under `internal/nova/**`. A successful run takes ~3-4 minutes
and ends with `libvirt smoke test PASSED` in the job log.

If it hangs at "waiting for ACTIVE", the most common causes are:

- `/dev/kvm` permissions — check `ls -l /dev/kvm` is `crw-rw----` and the
  runner user is in the `kvm` group
- Out of memory — the test uses the `m1.tiny` flavor (512 MiB) but libvirt
  also reserves overhead. Need at least 2 GiB free RAM
- AppArmor or SELinux blocking libvirt — check `dmesg | grep -i denied`

## Security notes

- This runner executes arbitrary code from PRs targeted at `main`. Treat the
  host as compromised-by-default. Run it in a dedicated VM or LXC container,
  not on a host with anything sensitive.
- The `NOPASSWD` sudo entry is scoped to the exact path of the smoke test
  script. Don't broaden it.
- Disable the runner when not actively needed by stopping the service:
  `sudo /home/ghrunner/actions-runner/svc.sh stop`.
- Rotate the registration token after provisioning. GitHub treats it as
  short-lived but the runner's own `.runner` config file is the long-lived
  credential — protect that directory.

## Decommissioning

```bash
sudo -iu ghrunner
cd ~/actions-runner
sudo ./svc.sh stop && sudo ./svc.sh uninstall
./config.sh remove --token <REMOVAL_TOKEN_FROM_GITHUB_UI>
```

Then delete the user and the sudoers drop-in:

```bash
sudo userdel -r ghrunner
sudo rm /etc/sudoers.d/ghrunner-libvirt
```
