import { URL } from "../types";
/**
 * @additionalProperties false
 */
export type HomepageConfig = {
  /** Whether show in dashboard
   *
   * @default true
   */
  show?: boolean;
  name?: string;
  icon?: URL | WalkxcodeIcon | ExternalIcon | TargetRelativeIconPath;
  description?: string;
  url?: URL;
  category?: string;
  widget_config?: {
    [key: string]: any;
  };
};
/** Walkxcode icon
 *
 * @pattern ^(png|svg|webp)\/[\w\d\-_]+\.\1
 * @type string
 */
export type WalkxcodeIcon = string & {};
export type ExternalIcon = `@${"selfhst" | "walkxcode"}/${string}.${string}`;
export type TargetRelativeIconPath = `@target/${string}` | `/${string}`;
