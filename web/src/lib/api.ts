import { getLuneToken, logout } from "./auth";

async function luneRequest<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const headers: Record<string, string> = {
    Authorization: `Bearer ${getLuneToken()}`,
  };
  if (body !== undefined) {
    headers["Content-Type"] = "application/json";
  }
  const res = await fetch(path, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  if (res.status === 401) {
    logout();
    throw new Error("unauthorized");
  }
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || `${res.status} ${res.statusText}`);
  }
  return res.json();
}

export function luneGet<T>(path: string) {
  return luneRequest<T>("GET", path);
}

export function lunePost<T>(path: string, body?: unknown) {
  return luneRequest<T>("POST", path, body);
}

export function lunePut<T>(path: string, body?: unknown) {
  return luneRequest<T>("PUT", path, body);
}

export function luneDelete<T>(path: string) {
  return luneRequest<T>("DELETE", path);
}
