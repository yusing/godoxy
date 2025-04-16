import { Duration, URI } from "../types";
export declare const STOP_METHODS: readonly ["pause", "stop", "kill"];
export type StopMethod = (typeof STOP_METHODS)[number];
export declare const STOP_SIGNALS: readonly [
  "",
  "SIGINT",
  "SIGTERM",
  "SIGHUP",
  "SIGQUIT",
  "INT",
  "TERM",
  "HUP",
  "QUIT",
];
export type Signal = (typeof STOP_SIGNALS)[number];
export type IdleWatcherConfig = {
  idle_timeout?: Duration;
  /** Wake timeout
   *
   * @default 30s
   */
  wake_timeout?: Duration;
  /** Stop timeout
   *
   * @default 30s
   */
  stop_timeout?: Duration;
  /** Stop method
   *
   * @default stop
   */
  stop_method?: StopMethod;
  stop_signal?: Signal;
  start_endpoint?: URI;
};
