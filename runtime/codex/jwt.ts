import type { CodexAuthJson, CodexAuthMetadata } from "./types.js";

export interface JwtClaims {
  exp?: number;
  email?: string;
  account_id?: string;
  plan_type?: string;
  [key: string]: unknown;
}

export function base64UrlDecode(input: string): string {
  const normalized = input.replace(/-/g, "+").replace(/_/g, "/");
  const padded = normalized.padEnd(normalized.length + ((4 - (normalized.length % 4)) % 4), "=");
  return Buffer.from(padded, "base64").toString("utf8");
}

export function parseJwtPayload(token?: string): JwtClaims | null {
  if (!token || typeof token !== "string") return null;
  const parts = token.split(".");
  if (parts.length < 2) return null;
  try {
    const parsed = JSON.parse(base64UrlDecode(parts[1]));
    return parsed && typeof parsed === "object" ? parsed : null;
  } catch {
    return null;
  }
}

export function jwtExpiresAt(token?: string): Date | undefined {
  const exp = parseJwtPayload(token)?.exp;
  return typeof exp === "number" && Number.isFinite(exp) ? new Date(exp * 1000) : undefined;
}

export function isJwtExpiringSoon(token?: string, skewMs = 5 * 60 * 1000): boolean {
  const expiresAt = jwtExpiresAt(token);
  if (!expiresAt) return true;
  return expiresAt.getTime() - Date.now() <= skewMs;
}

export function metadataFromAuth(auth: CodexAuthJson | null | undefined): CodexAuthMetadata {
  const accessClaims = parseJwtPayload(auth?.tokens?.access_token);
  const idClaims = parseJwtPayload(auth?.tokens?.id_token);
  const claims = { ...(accessClaims || {}), ...(idClaims || {}) } as JwtClaims;
  const accountId = stringClaim(claims.account_id) || stringClaim(claims.accountId) || stringClaim(auth?.tokens?.account_id);
  const profile = claims["https://api.openai.com/profile"] as Record<string, unknown> | undefined;
  const accountEmail = stringClaim(claims.email) || stringClaim(profile?.email);
  const planType = stringClaim(claims.plan_type) || stringClaim(claims.planType);
  return {
    ...(accountEmail ? { accountEmail } : {}),
    ...(accountId ? { accountId } : {}),
    ...(planType ? { planType } : {}),
    ...(auth?.last_refresh ? { updatedAt: String(auth.last_refresh) } : {}),
  };
}

function stringClaim(value: unknown): string | undefined {
  return typeof value === "string" && value.trim() ? value : undefined;
}
