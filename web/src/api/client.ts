import { useSettingsStore } from "@/stores/settings";
import type { APIErrorBody } from "./types";

export class APIError extends Error {
  status: number;
  code: string;
  requestId?: string;

  constructor(status: number, code: string, message: string, requestId?: string) {
    super(message);
    this.name = "APIError";
    this.status = status;
    this.code = code;
    this.requestId = requestId;
  }
}

function hint(status: number, code: string, message: string): string {
  if (status === 401 || code === "unauthorized") {
    return "API Key 无效，请到「连接设置」检查 X-API-Key";
  }
  if (status === 503) {
    return "数据库未启用（最小化部署）。会话 / Agent / 语料 / 日志不可用";
  }
  if (code === "uid_required") {
    return "缺少 X-User-Id，请到「连接设置」填写用户 ID";
  }
  return message || `HTTP ${status}`;
}

export function apiURL(path: string): string {
  const settings = useSettingsStore();
  const base = settings.apiBase.replace(/\/+$/, "");
  const p = path.startsWith("/") ? path : `/${path}`;
  return base ? `${base}${p}` : p;
}

export function authHeaders(extra?: HeadersInit): Headers {
  const settings = useSettingsStore();
  const h = new Headers(extra);
  if (settings.userId) h.set("X-User-Id", settings.userId);
  return h;
}

async function parseError(res: Response): Promise<APIError> {
  let body: APIErrorBody = {};
  try {
    body = (await res.json()) as APIErrorBody;
  } catch {
    /* ignore */
  }
  const code = body.error?.code || "";
  const message = body.error?.message || res.statusText;
  return new APIError(res.status, code, hint(res.status, code, message), body.request_id);
}

export async function requestJSON<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = authHeaders(init.headers);
  if (init.body && !headers.has("Content-Type") && !(init.body instanceof FormData)) {
    headers.set("Content-Type", "application/json");
  }
  const res = await fetch(apiURL(path), { ...init, headers });
  if (res.status === 204) {
    return undefined as T;
  }
  if (!res.ok) {
    throw await parseError(res);
  }
  return (await res.json()) as T;
}

export async function downloadFile(path: string, fallbackName: string): Promise<void> {
  const res = await fetch(apiURL(path), { headers: authHeaders() });
  if (!res.ok) {
    throw await parseError(res);
  }
  const blob = await res.blob();
  const name = filenameFromDisposition(res.headers.get("Content-Disposition")) || fallbackName || "download";
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

function filenameFromDisposition(header: string | null): string {
  if (!header) return "";
  const star = /filename\*=UTF-8''([^;]+)/i.exec(header);
  if (star?.[1]) {
    try {
      return decodeURIComponent(star[1]);
    } catch {
      return star[1];
    }
  }
  const plain = /filename="([^"]+)"/i.exec(header);
  return plain?.[1] || "";
}

export async function requestForm<T>(path: string, form: FormData, method = "POST"): Promise<T> {
  const res = await fetch(apiURL(path), { method, headers: authHeaders(), body: form });
  if (!res.ok) {
    throw await parseError(res);
  }
  return (await res.json()) as T;
}

export function formatAPIError(err: unknown): string {
  if (err instanceof APIError) {
    const rid = err.requestId ? `（request_id: ${err.requestId}）` : "";
    return `${err.message}${rid}`;
  }
  if (err instanceof Error) return err.message;
  return String(err);
}
