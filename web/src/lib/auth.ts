const LUNE_TOKEN_KEY = "lune_admin_token";
const BACKEND_TOKEN_KEY = "backend_token";

export function getLuneToken(): string {
  return sessionStorage.getItem(LUNE_TOKEN_KEY) ?? "";
}

export function setLuneToken(token: string) {
  sessionStorage.setItem(LUNE_TOKEN_KEY, token);
}

export function getBackendToken(): string {
  return sessionStorage.getItem(BACKEND_TOKEN_KEY) ?? "";
}

export function setBackendToken(token: string) {
  sessionStorage.setItem(BACKEND_TOKEN_KEY, token);
}

export function isAuthenticated(): boolean {
  return getLuneToken() !== "" && getBackendToken() !== "";
}

export function logout() {
  sessionStorage.removeItem(LUNE_TOKEN_KEY);
  sessionStorage.removeItem(BACKEND_TOKEN_KEY);
  window.location.href = "/admin/login";
}
