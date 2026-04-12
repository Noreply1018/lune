const LUNE_TOKEN_KEY = "lune_admin_token";

export function getLuneToken(): string {
  return sessionStorage.getItem(LUNE_TOKEN_KEY) ?? "";
}

export function setLuneToken(token: string) {
  sessionStorage.setItem(LUNE_TOKEN_KEY, token);
}

export function isAuthenticated(): boolean {
  return getLuneToken() !== "";
}

export function logout() {
  sessionStorage.removeItem(LUNE_TOKEN_KEY);
  window.location.href = "/admin/login";
}
