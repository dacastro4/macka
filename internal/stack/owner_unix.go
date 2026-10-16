//go:build unix

package stack

import (
	"os"
	"syscall"
)

// ResolveUser returns the configured user, or the owner of the stack directory.
// Containers must run as the data owner; never assume uid 1000 (COMPATIBILITY.md).
func (s *State) ResolveUser() (User, error) {
	if s.User != nil {
		return *s.User, nil
	}
	fi, err := os.Stat(s.Dir)
	if err != nil {
		return User{}, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return User{}, ErrNoUser
	}
	return User{UID: int(st.Uid), GID: int(st.Gid)}, nil
}
