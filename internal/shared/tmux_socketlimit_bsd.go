//go:build darwin

package shared

// maxTmuxSocketPath is the longest tmux socket path the kernel will bind.
// Darwin sizes sockaddr_un.sun_path at 104 bytes, one of which is the NUL
// terminator. Verified empirically: a 103-byte socket path connects, 104 fails.
const maxTmuxSocketPath = 103
