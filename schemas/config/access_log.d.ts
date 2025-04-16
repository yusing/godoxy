import { CIDR, HTTPHeader, HTTPMethod, StatusCodeRange, URI } from "../types";
export declare const ACCESS_LOG_FORMATS: readonly [
  "combined",
  "common",
  "json",
];
export type AccessLogFormat = (typeof ACCESS_LOG_FORMATS)[number];
export type AccessLogConfig = {
  /**
   * The size of the buffer.
   *
   * @minimum 0
   * @default 65536
   * @TJS-type integer
   */
  buffer_size?: number;
  /** The format of the access log.
   *
   * @default "combined"
   */
  format?: AccessLogFormat;
  path: URI;
  filters?: AccessLogFilters;
  fields?: AccessLogFields;
};
export type AccessLogFilter<T> = {
  /** Whether the filter is negative.
   *
   * @default false
   */
  negative?: boolean;
  values: T[];
};
export type AccessLogFilters = {
  status_code?: AccessLogFilter<StatusCodeRange>;
  method?: AccessLogFilter<HTTPMethod>;
  host?: AccessLogFilter<string>;
  headers?: AccessLogFilter<HTTPHeader>;
  cidr?: AccessLogFilter<CIDR>;
};
export declare const ACCESS_LOG_FIELD_MODES: readonly [
  "keep",
  "drop",
  "redact",
];
export type AccessLogFieldMode = (typeof ACCESS_LOG_FIELD_MODES)[number];
export type AccessLogField = {
  default?: AccessLogFieldMode;
  config: {
    [key: string]: AccessLogFieldMode;
  };
};
export type AccessLogFields = {
  header?: AccessLogField;
  query?: AccessLogField;
  cookie?: AccessLogField;
};
