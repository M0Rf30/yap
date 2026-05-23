#!/usr/bin/env bash
set -euo pipefail

echo "==> E2E: pure-Go RPM install on $(. /etc/os-release && echo "$PRETTY_NAME")"

# 1. Fetch a small RPM via dnf download
RPM_PATH="/tmp/yap-e2e-tree.rpm"
if [[ ! -f "$RPM_PATH" ]]; then
    echo "==> Fetching tree RPM via dnf download..."
    dnf -y install --setopt=install_weak_deps=False dnf-plugins-core >/tmp/plugin.log 2>&1 || (cat /tmp/plugin.log; exit 1)
    cd /tmp
    dnf download tree --destdir=/tmp >/tmp/download.log 2>&1 || (cat /tmp/download.log; exit 1)
    cd -
    DOWNLOADED=$(ls -1 /tmp/tree-*.rpm 2>/dev/null | head -1)
    [[ -n "$DOWNLOADED" ]] || { echo "FAIL: dnf download did not produce tree RPM"; exit 1; }
    mv "$DOWNLOADED" "$RPM_PATH"
fi
echo "==> Got $(stat -c '%s' "$RPM_PATH") bytes from $RPM_PATH"

# 2. Baseline: assert tree is NOT installed by system rpm
if rpm -q tree >/dev/null 2>&1; then
    echo "FAIL: 'tree' is already installed; container not clean"
    exit 1
fi
if test -x /usr/bin/tree; then
    echo "FAIL: /usr/bin/tree already exists"
    exit 1
fi
echo "==> Baseline check passed: tree not installed"

# 3. Install via YAP
echo "==> Installing via yap install..."
./yap install "$RPM_PATH"

# 4. Assert files placed
if ! test -x /usr/bin/tree; then
    echo "FAIL: /usr/bin/tree not installed"
    exit 1
fi
echo "==> Binary installed: /usr/bin/tree"

if ! /usr/bin/tree -L 1 /etc/yum.repos.d > /tmp/tree-out.txt 2>&1; then
    echo "FAIL: tree binary not functional"
    cat /tmp/tree-out.txt
    exit 1
fi
if ! grep -q "Rocky-AppStream.repo\|rocky" /tmp/tree-out.txt; then
    echo "FAIL: tree output unexpected"
    cat /tmp/tree-out.txt
    exit 1
fi
echo "==> Binary functional: tree -L 1 /etc/yum.repos.d works"

# 5. Assert YAP state DB recorded the install
DB="/var/lib/yap/installed.db"
if ! test -f "$DB"; then
    echo "FAIL: yapdb not created at $DB"
    exit 1
fi
echo "==> YAP state DB exists: $DB"

NAME=$(sqlite3 "$DB" "SELECT name FROM packages WHERE name='tree'" || echo "")
if [[ "$NAME" != "tree" ]]; then
    echo "FAIL: tree not recorded in yapdb (got: '$NAME')"
    exit 1
fi
echo "==> Package recorded in yapdb"

FILES=$(sqlite3 "$DB" "SELECT COUNT(*) FROM files WHERE package_id=(SELECT id FROM packages WHERE name='tree')" || echo "0")
if [[ "$FILES" -le 0 ]]; then
    echo "FAIL: no files recorded for tree (count: $FILES)"
    exit 1
fi
echo "==> Files recorded in yapdb: $FILES files"

TREE_BIN_RECORDED=$(sqlite3 "$DB" "SELECT path FROM files WHERE path LIKE '%/bin/tree' AND package_id=(SELECT id FROM packages WHERE name='tree')" || echo "")
if [[ -z "$TREE_BIN_RECORDED" ]]; then
    echo "FAIL: /usr/bin/tree path not recorded in yapdb"
    exit 1
fi
echo "==> Binary path recorded in yapdb: $TREE_BIN_RECORDED"

# 6. Assert system rpmdb was NOT touched
if rpm -q tree >/dev/null 2>&1; then
    echo "FAIL: system rpmdb knows about tree (we shouldn't have written there)"
    exit 1
fi
echo "==> System rpmdb untouched (tree not in rpm -q)"

# 7. Cleanup
echo "==> Cleaning up..."
rm -f /usr/bin/tree /usr/share/man/man1/tree.1.gz
rm -f "$DB"

echo "==> E2E PASS"
