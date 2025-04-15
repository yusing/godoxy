/**
 * @type "null"
 */
export type Null = null;
export type Nullable<T> = T | Null;
export type NullOrEmptyMap = {} | Null;
export declare const HTTP_METHODS: readonly [
  "GET",
  "POST",
  "PUT",
  "PATCH",
  "DELETE",
  "CONNECT",
  "HEAD",
  "OPTIONS",
  "TRACE",
];
export type HTTPMethod = (typeof HTTP_METHODS)[number];
/**
 * HTTP Header
 * @pattern ^[a-zA-Z0-9\-]+$
 * @type string
 */
export type HTTPHeader = string & {};
/**
 * HTTP Query
 * @pattern ^[a-zA-Z0-9\-_]+$
 * @type string
 */
export type HTTPQuery = string & {};
/**
 * HTTP Cookie
 * @pattern ^[a-zA-Z0-9\-_]+$
 * @type string
 */
export type HTTPCookie = string & {};
export type StatusCode = number | `${number}`;
export type StatusCodeRange = number | `${number}` | `${number}-${number}`;
/**
 * @pattern ^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z0-9][a-z0-9-]{0,61}[a-z0-9]$
 */
export type DomainName = string & {};
/**
 * @pattern ^(\*\.)?(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z0-9][a-z0-9-]{0,61}[a-z0-9]$
 */
export type DomainOrWildcard = string & {};
/**
 * @format hostname
 * @type string
 */
export type Hostname = string & {};
/**
 * @format ipv4
 * @type string
 */
export type IPv4 = string & {};
/**
 * @format ipv6
 * @type string
 */
export type IPv6 = string & {};
export type CIDR =
  | `${number}.${number}.${number}.${number}`
  | `${string}:${string}:${string}:${string}:${string}:${string}:${string}:${string}`
  | `${number}.${number}.${number}.${number}/${number}`
  | `::${number}`
  | `${string}::/${number}`
  | `${string}:${string}::/${number}`;
/**
 * @type integer
 * @minimum 0
 * @maximum 65535
 */
export type Port = number | `${number}`;
/**
 * @pattern ^\d+:\d+$
 * @type string
 */
export type StreamPort = string & {};
/**
 * @format email
 * @type string
 */
export type Email = string & {};
/**
 * @format uri
 * @type string
 */
export type URL = string & {};
/**
 * @format uri-reference
 * @type string
 */
export type URI = string & {};
/**
 * @pattern ^(?:([A-Z]+) )?(?:([a-zA-Z0-9.-]+)\\/)?(\\/[^\\s]*)$
 * @type string
 */
export type PathPattern = string & {};
/**
 * @pattern ^([0-9]+(ms|s|m|h))+$
 * @type string
 */
export type Duration = string & {};
/**
 * @format date-time
 * @type string
 */
export type DateTime = string & {};
