/**
 * Role hints for UI only (server enforces authorization).
 * When using trusted headers, map X-OpenSandbox-Roles: operator | read_only.
 */
export function parseRoleFromEnv(): "operator" | "read_only" {
  const r = (import.meta.env.VITE_UI_ROLE as string | undefined)?.toLowerCase() ?? "operator";
  if (r.includes("read")) {
    return "read_only";
  }
  return "operator";
}

export function canMutate(role: "operator" | "read_only"): boolean {
  return role === "operator";
}
