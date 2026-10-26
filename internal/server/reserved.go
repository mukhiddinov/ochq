package server

// reserved lists subdomains no tunnel may claim: names of our own services
// (tunnel is the control endpoint, pkg the planned package host), names a
// visitor would trust (www, login, admin, ...), and demo, which the ochq.uz
// landing page shows in its animation and must not lead to someone's page.
var reserved = map[string]struct{}{
	"admin": {}, "api": {}, "app": {}, "blog": {}, "dashboard": {},
	"demo": {}, "docs": {}, "login": {}, "mail": {}, "ochq": {},
	"pkg": {}, "status": {}, "tunnel": {}, "www": {},
}

// isReserved expects a subdomain already normalized by proto.NormalizeSubdomain.
func isReserved(sub string) bool {
	_, ok := reserved[sub]
	return ok
}
