import { AccessLogConfig } from "../config/access_log";
import { accessLogExamples } from "../config/entrypoint";
import { MiddlewaresMap } from "../middlewares/middlewares";
import {
  Duration,
  Hostname,
  IPv4,
  IPv6,
  PathPattern,
  Port,
  StreamPort,
} from "../types";
import { HealthcheckConfig } from "./healthcheck";
import { HomepageConfig } from "./homepage";
import { LoadBalanceConfig } from "./loadbalance";
export const PROXY_SCHEMES = ["http", "https"] as const;
export const STREAM_SCHEMES = ["tcp", "udp"] as const;

export type ProxyScheme = (typeof PROXY_SCHEMES)[number];
export type StreamScheme = (typeof STREAM_SCHEMES)[number];

export type Route = ReverseProxyRoute | FileServerRoute | StreamRoute;
export type Routes = {
  [key: string]: Route;
};

export type ReverseProxyRoute = {
  /** Alias (subdomain or FDN)
   * @minLength 1
   */
  alias?: string;
  /** Proxy scheme
   *
   * @default http
   */
  scheme?: ProxyScheme;
  /** Proxy host
   *
   * @default localhost
   */
  host?: Hostname | IPv4 | IPv6;
  /** Proxy port
   *
   * @default 80
   */
  port?: Port;
  /** Skip TLS verification
   *
   * @default false
   */
  no_tls_verify?: boolean;
  /** Response header timeout
   *
   * @default 60s
   */
  response_header_timeout?: Duration;
  /** Path patterns (only patterns that match will be proxied).
   *
   * See https://pkg.go.dev/net/http#hdr-Patterns-ServeMux
   */
  path_patterns?: PathPattern[];
  /** Healthcheck config */
  healthcheck?: HealthcheckConfig;
  /** Load balance config */
  load_balance?: LoadBalanceConfig;
  /** Middlewares */
  middlewares?: MiddlewaresMap;
  /** Homepage config
   *
   * @examples require(".").homepageExamples
   */
  homepage?: HomepageConfig;
  /** Access log config
   *
   * @examples require(".").accessLogExamples
   */
  access_log?: AccessLogConfig;
};

export type FileServerRoute = {
  /** Alias (subdomain or FDN)
   * @minLength 1
   */
  alias?: string;
  scheme: "fileserver";
  /* File server root path */
  root: string;
  /** Path patterns (only patterns that match will be proxied).
   *
   * See https://pkg.go.dev/net/http#hdr-Patterns-ServeMux
   */
  path_patterns?: PathPattern[];
  /** Middlewares */
  middlewares?: MiddlewaresMap;
  /** Homepage config
   *
   * @examples require(".").homepageExamples
   */
  homepage?: HomepageConfig;
  /** Access log config
   *
   * @examples require(".").accessLogExamples
   */
  access_log?: AccessLogConfig;
  /** Healthcheck config */
  healthcheck?: HealthcheckConfig;
};

export type StreamRoute = {
  /** Alias (subdomain or FDN)
   * @minLength 1
   */
  alias?: string;
  /** Stream scheme
   *
   * @default tcp
   */
  scheme?: StreamScheme;
  /** Stream host
   *
   * @default localhost
   */
  host?: Hostname | IPv4 | IPv6;
  /* Stream port */
  port: StreamPort;
  /** Healthcheck config */
  healthcheck?: HealthcheckConfig;
};

export const homepageExamples = [
  {
    name: "Sonarr",
    icon: "png/sonarr.png",
    category: "Arr suite",
  },
  {
    name: "App",
    icon: "@target/favicon.ico",
  },
];

export const loadBalanceExamples = [
  {
    link: "flaresolverr",
    mode: "round_robin",
  },
  {
    link: "service.domain.com",
    mode: "ip_hash",
    config: {
      header: "X-Real-IP",
    },
  },
];

export { accessLogExamples };
