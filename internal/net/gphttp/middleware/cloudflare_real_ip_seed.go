package middleware

//go:generate go run ../../../../cmd/cloudflare-cidrs-gen -out cloudflare_real_ip_seed_gen.go

var generatedCloudflareCIDRsRaw []byte
