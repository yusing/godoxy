# End-to-end tests

To run the browser E2E suite, run from the repository root:

```sh
shadowtree test-browser
```

Use a checkout with its submodules initialized, Bun, Docker, and
`agent-browser` with Chromium installed. IPv6 loopback must be available for
the HTTPS GoDoxy fixture. The recipe rebuilds the embedded WebUI and starts
a disposable Pocket ID container from a pinned image. It does not need an
existing Pocket ID account or GoDoxy configuration.

The first test covers logout. It checks for a fresh Pocket ID sign-in and
verifies that username/password logout works with a stale CSRF cookie.
