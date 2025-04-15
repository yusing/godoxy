import { Duration, URI } from "../types";
/**
 * @additionalProperties false
 */
export type HealthcheckConfig = {
  /** Disable healthcheck
   *
   * @default false
   */
  disable?: boolean;
  /** Healthcheck path
   *
   * @default /
   */
  path?: URI;
  /**
   * Use GET instead of HEAD
   *
   * @default false
   */
  use_get?: boolean;
  /** Healthcheck interval
   *
   * @default 5s
   */
  interval?: Duration;
  /** Healthcheck timeout
   *
   * @default 5s
   */
  timeout?: Duration;
};
