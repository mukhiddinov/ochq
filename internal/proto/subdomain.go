package proto

import (
	"errors"
	"strings"
)

const (
	MinSubdomainLen = 3
	MaxSubdomainLen = 63 // DNS label limit
)

var ErrInvalidSubdomain = errors.New("proto: invalid subdomain")

// NormalizeSubdomain lowercases s and checks that it is a valid single
// DNS label: [a-z0-9-], no leading or trailing hyphen.
func NormalizeSubdomain(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) < MinSubdomainLen || len(s) > MaxSubdomainLen {
		return "", ErrInvalidSubdomain
	}
	if s[0] == '-' || s[len(s)-1] == '-' {
		return "", ErrInvalidSubdomain
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return "", ErrInvalidSubdomain
		}
	}
	return s, nil
}
