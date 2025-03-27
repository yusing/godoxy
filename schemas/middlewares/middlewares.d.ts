import * as types from "../types";
export type KeyOptMapping<T extends {
    use: string;
}> = {
    [key in T["use"]]?: Omit<T, "use">;
};
export declare const ALL_MIDDLEWARES: readonly ["ErrorPage", "RedirectHTTP", "SetXForwarded", "HideXForwarded", "CIDRWhitelist", "CloudflareRealIP", "ModifyRequest", "ModifyResponse", "OIDC", "RateLimit", "RealIP"];
/**
 * @type object
 * @patternProperties {"^.*@file$": {"type": "null"}}
 */
export type MiddlewareFileRef = {
    [key: `${string}@file`]: null;
};
export type MiddlewaresMap = (KeyOptMapping<CustomErrorPage> & KeyOptMapping<RedirectHTTP> & KeyOptMapping<SetXForwarded> & KeyOptMapping<HideXForwarded> & KeyOptMapping<CIDRWhitelist> & KeyOptMapping<CloudflareRealIP> & KeyOptMapping<ModifyRequest> & KeyOptMapping<ModifyResponse> & KeyOptMapping<OIDC> & KeyOptMapping<RateLimit> & KeyOptMapping<RealIP>) | MiddlewareFileRef;
export type MiddlewareComposeMap = CustomErrorPage | RedirectHTTP | SetXForwarded | HideXForwarded | CIDRWhitelist | CloudflareRealIP | ModifyRequest | ModifyResponse | OIDC | RateLimit | RealIP;
export type CustomErrorPage = {
    use: "error_page" | "errorPage" | "ErrorPage" | "custom_error_page" | "customErrorPage" | "CustomErrorPage";
};
export type RedirectHTTP = {
    use: "redirect_http" | "redirectHTTP" | "RedirectHTTP";
    /** Bypass redirect */
    bypass?: {
        /** Bypass redirect for user agents */
        user_agents?: string[];
    };
};
export type SetXForwarded = {
    use: "set_x_forwarded" | "setXForwarded" | "SetXForwarded";
};
export type HideXForwarded = {
    use: "hide_x_forwarded" | "hideXForwarded" | "HideXForwarded";
};
export type CIDRWhitelist = {
    use: "cidr_whitelist" | "cidrWhitelist" | "CIDRWhitelist";
    allow: types.CIDR[];
    /** HTTP status code when blocked
     *
     * @default 403
     */
    status_code?: types.StatusCode;
    /** HTTP status code when blocked (alias of status_code)
     *
     * @default 403
     */
    status?: types.StatusCode;
    /** Error message when blocked
     *
     * @default "IP not allowed"
     */
    message?: string;
};
export type CloudflareRealIP = {
    use: "cloudflare_real_ip" | "cloudflareRealIp" | "CloudflareRealIP";
    /** Recursively resolve the IP
     *
     * @default false
     */
    recursive?: boolean;
};
export type ModifyRequest = {
    use: "request" | "Request" | "modify_request" | "modifyRequest" | "ModifyRequest";
    /** Set HTTP headers */
    set_headers?: {
        [key: types.HTTPHeader]: string;
    };
    /** Add HTTP headers */
    add_headers?: {
        [key: types.HTTPHeader]: string;
    };
    /** Hide HTTP headers */
    hide_headers?: types.HTTPHeader[];
    /** Add prefix to request URL */
    add_prefix?: string;
};
export type ModifyResponse = {
    use: "response" | "Response" | "modify_response" | "modifyResponse" | "ModifyResponse";
    /** Set HTTP headers */
    set_headers?: {
        [key: types.HTTPHeader]: string;
    };
    /** Add HTTP headers */
    add_headers?: {
        [key: types.HTTPHeader]: string;
    };
    /** Hide HTTP headers */
    hide_headers?: types.HTTPHeader[];
};
export type OIDC = {
    use: "oidc" | "OIDC";
    /** Allowed users
     *
     * @minItems 1
     */
    allowed_users?: string[];
    /** Allowed groups
     *
     * @minItems 1
     */
    allowed_groups?: string[];
};
export type RateLimit = {
    use: "rate_limit" | "rateLimit" | "RateLimit";
    /** Average number of requests allowed in a period
     *
     * @min 1
     */
    average: number;
    /** Maximum number of requests allowed in a period
     *
     * @min 1
     */
    burst: number;
    /** Duration of the rate limit
     *
     * @default 1s
     */
    period?: types.Duration;
};
export type RealIP = {
    use: "real_ip" | "realIP" | "RealIP";
    /** Header to get the client IP from
     *
     * @default "X-Real-IP"
     */
    header?: types.HTTPHeader;
    from: types.CIDR[];
    /** Recursive resolve the IP
     *
     * @default false
     */
    recursive?: boolean;
};
