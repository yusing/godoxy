package middleware

import _ "embed"

//go:embed cloudflare_real_ip_seed.txt
var embeddedCloudflareCIDRsRaw []byte
