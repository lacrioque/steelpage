package auth

import "strings"

// publicURL builds an absolute URL the operator can give to users (e.g. in a
// password-reset email). Reads server.base_url live so admins can change it
// without a restart.
func (s *Service) publicURL(path string) string {
	live := s.live()
	base := live.Server.BaseURL
	if base == "" {
		base = "http://" + live.Server.Bind
	}
	return strings.TrimRight(base, "/") + path
}
