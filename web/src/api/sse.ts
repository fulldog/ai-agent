import { apiURL, authHeaders, APIError } from "./client";

export interface SSEEvent {
  event: string;
  data: Record<string, unknown>;
}

function parseBlock(block: string): SSEEvent | null {
  let event = "message";
  const dataLines: string[] = [];
  for (const line of block.split("\n")) {
    if (line.startsWith("event:")) {
      event = line.slice(6).trim();
    } else if (line.startsWith("data:")) {
      dataLines.push(line.slice(5).trimStart());
    }
  }
  if (dataLines.length === 0) return null;
  const raw = dataLines.join("\n");
  let data: Record<string, unknown> = {};
  try {
    data = JSON.parse(raw) as Record<string, unknown>;
  } catch {
    data = { raw };
  }
  return { event, data };
}

export async function postSSE(
  path: string,
  body: unknown,
  onEvent: (ev: SSEEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  const headers = authHeaders({ Accept: "text/event-stream" });
  headers.set("Content-Type", "application/json");
  const res = await fetch(apiURL(path), {
    method: "POST",
    headers,
    body: JSON.stringify(body),
    signal,
  });
  if (!res.ok) {
    let message = res.statusText;
    let code = "";
    let requestId: string | undefined;
    try {
      const j = (await res.json()) as { error?: { code?: string; message?: string }; request_id?: string };
      code = j.error?.code || "";
      message = j.error?.message || message;
      requestId = j.request_id;
    } catch {
      /* ignore */
    }
    if (res.status === 401) message = "API Key 无效，请到「连接设置」检查 X-API-Key";
    if (res.status === 503) message = "数据库未启用（最小化部署）。会话 / Agent / 语料 / 日志不可用";
    if (code === "uid_required") message = "缺少 X-User-Id，请到「连接设置」填写用户 ID";
    throw new APIError(res.status, code, message, requestId);
  }
  if (!res.body) {
    throw new Error("响应没有可读流");
  }
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buf = "";
  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buf += decoder.decode(value, { stream: true });
    buf = buf.replace(/\r\n/g, "\n");
    let idx: number;
    while ((idx = buf.indexOf("\n\n")) >= 0) {
      const block = buf.slice(0, idx);
      buf = buf.slice(idx + 2);
      const ev = parseBlock(block);
      if (ev) onEvent(ev);
    }
  }
}
