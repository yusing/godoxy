import { DomainName } from "../types";
import { AutocertConfig } from "./autocert";
import { EntrypointConfig } from "./entrypoint";
import { HomepageConfig } from "./homepage";
import { Providers } from "./providers";
export type Config = {
  /** Optional autocert configuration
   *
   * @examples require(".").autocertExamples
   */
  autocert?: AutocertConfig;
  entrypoint?: EntrypointConfig;
  providers: Providers;
  /** Optional list of domains to match
   *
   * @minItems 1
   * @examples require(".").matchDomainsExamples
   */
  match_domains?: DomainName[];
  homepage?: HomepageConfig;
  /**
   * Optional timeout before shutdown
   * @default 3
   * @minimum 1
   */
  timeout_shutdown?: number;
};
export declare const autocertExamples: (
  | {
      provider: string;
      email?: undefined;
      domains?: undefined;
      options?: undefined;
    }
  | {
      provider: string;
      email: string;
      domains: string[];
      options: {
        auth_token: string;
        client_id?: undefined;
        email?: undefined;
        password?: undefined;
      };
    }
  | {
      provider: string;
      email: string;
      domains: string[];
      options: {
        client_id: string;
        email: string;
        password: string;
        auth_token?: undefined;
      };
    }
)[];
export declare const matchDomainsExamples: readonly [
  "example.com",
  "*.example.com",
];
