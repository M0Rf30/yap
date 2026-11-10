# dpkg status fixture

`status` is a real `/var/lib/dpkg/status` (dpkg 1.22.22, Debian 13.7,
`debian:stable-slim` plus a few installed, held, unpacked and removed packages),
the one used by the `pkgformat` status reader tests. It has 91 stanzas, with
multi-line descriptions, `" ."` paragraph markers and `Conffiles` lists.

`TestStatusRoundTripRealFile` reads it with `readDpkgStatus`, writes it back with
`writeDpkgStatus` and checks that each stanza has the same lines.
