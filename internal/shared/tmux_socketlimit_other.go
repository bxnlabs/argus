//go:build !darwin && !dragonfly && !freebsd && !netbsd && !openbsd

package shared

// maxTmuxSocketPath is the longest tmux socket path the kernel will bind.
// Linux sizes sockaddr_un.sun_path at 108 bytes, one of which is the NUL
// terminator. This is also the fallback for any other GOOS, so the constant
// is always defined and the package builds everywhere.
const maxTmuxSocketPath = 107
