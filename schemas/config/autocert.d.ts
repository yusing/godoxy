import { DomainOrWildcard, Email } from "../types";
export declare const AUTOCERT_PROVIDERS: readonly [
  "local",
  "cloudflare",
  "clouddns",
  "duckdns",
  "ovh",
  "porkbun",
];
export type AutocertProvider = (typeof AUTOCERT_PROVIDERS)[number];
export type AutocertConfig =
  | LocalOptions
  | CloudflareOptions
  | CloudDNSOptions
  | DuckDNSOptions
  | OVHOptionsWithAppKey
  | OVHOptionsWithOAuth2Config
  | PorkbunOptions;
export interface AutocertConfigBase {
  email: Email;
  domains: DomainOrWildcard[];
  cert_path?: string;
  key_path?: string;
}
export interface LocalOptions {
  provider: "local";
  cert_path?: string;
  key_path?: string;
  options?: {} | null;
}
export interface CloudflareOptions extends AutocertConfigBase {
  provider: "cloudflare";
  options: {
    auth_token: string;
  };
}
export interface CloudDNSOptions extends AutocertConfigBase {
  provider: "clouddns";
  options: {
    client_id: string;
    email: Email;
    password: string;
  };
}
export interface DuckDNSOptions extends AutocertConfigBase {
  provider: "duckdns";
  options: {
    token: string;
  };
}
export interface PorkbunOptions extends AutocertConfigBase {
  provider: "porkbun";
  options: {
    api_key: string;
    secret_api_key: string;
  };
}
export declare const OVH_ENDPOINTS: readonly [
  "ovh-eu",
  "ovh-ca",
  "ovh-us",
  "kimsufi-eu",
  "kimsufi-ca",
  "soyoustart-eu",
  "soyoustart-ca",
];
export type OVHEndpoint = (typeof OVH_ENDPOINTS)[number];
export interface OVHOptionsWithAppKey extends AutocertConfigBase {
  provider: "ovh";
  options: {
    application_secret: string;
    consumer_key: string;
    api_endpoint?: OVHEndpoint;
    application_key: string;
  };
}
export interface OVHOptionsWithOAuth2Config extends AutocertConfigBase {
  provider: "ovh";
  options: {
    application_secret: string;
    consumer_key: string;
    api_endpoint?: OVHEndpoint;
    oauth2_config: {
      client_id: string;
      client_secret: string;
    };
  };
}
