# Server environment settings

GoDoxy-owned environment registry, typed reads, and safe startup diagnostics.

## Ownership

This is application policy, not part of goutils. Server consumers
read registered defaults here; the standalone CLI and shared utility libraries
continue using their generic environment accessors.

## Lookup and diagnostics

Settings resolve `GODOXY_`, then `GOPROXY_`, then the unprefixed name, taking the
first non-empty value. Unprefixed server settings remain supported but trigger
a migration warning. Diagnostics redact sensitive values and never log values
of unknown variables. Parsing failures omit the supplied value.

`HTTPS_ADDR` is the exception: the first present variable wins, even when empty.
An explicitly empty `GODOXY_HTTPS_ADDR` disables the shared HTTPS listener, HTTP/3
on that listener, and shared TCP SNI routing. An unset setting still defaults to
`:443` and requires an available certificate. Dedicated route listeners are not
disabled. `redirectHTTP` does not redirect when shared HTTPS is disabled. For direct
HTTP authentication, also set `GODOXY_API_JWT_SECURE=false`; keep secure cookies
enabled when clients connect through a TLS-terminating external proxy.

## Generated documentation

Update the registry and run `shadowtree gen-env-docs` to regenerate the example
and wiki environment table. `shadowtree check-env-docs` verifies drift.

## MaxMind downloads

`GODOXY_MAXMIND_COUNTRY_ONLY` defaults to `false`: MaxMind uses City databases.
Set it to `true` to select Country downloads and filenames instead; timezone
ACL rules need City data.
