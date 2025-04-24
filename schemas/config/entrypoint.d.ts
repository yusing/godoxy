import { MiddlewareCompose } from "../middlewares/middleware_compose";
import { AccessLogConfig } from "./access_log";
export type EntrypointConfig = {
  /** Entrypoint middleware configuration
   *
   * @examples require(".").middlewaresExamples
   */
  middlewares?: MiddlewareCompose;
  /** Entrypoint access log configuration
   *
   * @examples require(".").accessLogExamples
   */
  access_log?: AccessLogConfig;
};
export declare const accessLogExamples: readonly [
  {
    readonly path: "/var/log/access.log";
    readonly format: "combined";
    readonly filters: {
      readonly status_codes: {
        readonly values: readonly ["200-299"];
      };
    };
    readonly fields: {
      readonly headers: {
        readonly default: "keep";
        readonly config: {
          readonly foo: "redact";
        };
      };
    };
  },
];
export declare const middlewaresExamples: readonly [
  {
    readonly use: "RedirectHTTP";
  },
  {
    readonly use: "CIDRWhitelist";
    readonly allow: readonly [
      "127.0.0.1",
      "10.0.0.0/8",
      "172.16.0.0/12",
      "192.168.0.0/16",
    ];
    readonly status: 403;
    readonly message: "Forbidden";
  },
];
