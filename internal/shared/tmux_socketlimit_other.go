//go:build !darwin

package shared

// maxTmuxSocketPath is the longest tmux socket path the kernel will bind.
// Linux sizes sockaddr_un.sun_path at 108 bytes, one of which is the NUL
// terminator. The BSDs use 104 like Darwin; on those the check is merely
// looser than the kernel's, so an over-long path still fails — just with
// tmux's own "File name too long" instead of our message.
const maxTmuxSocketPath = 107
