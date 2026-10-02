# Country database fixture

`GeoIP2-Country-Test.mmdb` is MaxMind's synthetic test database, from
[MaxMind-DB revision 276926d](https://github.com/maxmind/MaxMind-DB/tree/276926d23b4109ca5452709bfb5931c338afb34c/test-data).
It is distributed under the accompanying MIT license.

ACL tests use `81.2.69.160` (GB), `175.16.199.1` (CN), and `2001:218::1`
(JP). These are fixture records, not assertions about live IP geolocation.
Country databases do not contain timezone data.
