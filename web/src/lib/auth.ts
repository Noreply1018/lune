const LUNE_TOKEN_KEY = "lune_admin_token";
const ONEAPI_TOKEN_KEY = "oneapi_token";

export function getLuneToken(): string {
  return sessionStorage.getItem(LUNE_TOKEN_KEY) ?? "";
}

export function setLuneToken(token: string) {
  sessionStorage.setItem(LUNE_TOKEN_KEY, token);
}

export function getOneapiToken(): string {
  return sessionStorage.getItem(ONEAPI_TOKEN_KEY) ?? "";
}

export function setOneapiToken(token: string) {
  sessionStorage.setItem(ONEAPI_TOKEN_KEY, token);
}

export function isAuthenticated(): boolean {
  return getLuneToken() !== "" && getOneapiToken() !== "";
}

export function logout() {
  sessionStorage.removeItem(LUNE_TOKEN_KEY);
  sessionStorage.removeItem(ONEAPI_TOKEN_KEY);
  window.location.href = "/admin/login";
}
