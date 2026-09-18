export interface APIErrorBody {
  error?: { code?: string; message?: string };
  request_id?: string;
}

export interface Usage {
  prompt_tokens?: number;
  completion_tokens?: number;
  total_tokens?: number;
}

export interface TokenBucket {
  calls: number;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
}

export interface TokenUsageItem {
  provider: string;
  model: string;
  all: TokenBucket;
  day: TokenBucket;
  week: TokenBucket;
  month: TokenBucket;
}

export interface TokenUsage {
  timezone?: string;
  day_from?: string;
  week_from?: string;
  month_from?: string;
  items: TokenUsageItem[];
  totals: Pick<TokenUsageItem, "all" | "day" | "week" | "month">;
  scope_admin?: boolean;
}

export interface Health {
  status: string;
  db: string;
  mode?: string;
}

export interface ProviderInfo {
  name: string;
  base_url: string;
  default_model: string;
  configured: boolean;
  enabled: boolean;
}

export interface Conversation {
  id: string;
  uid: string;
  title: string;
  system_prompt: string;
  corpus_id?: string;
  created_at: string;
  updated_at: string;
}

export interface Message {
  id: string;
  conversation_id: string;
  role: string;
  content: string;
  created_at: string;
  token_prompt?: number;
  token_completion?: number;
}

export interface Corpus {
  id: string;
  name: string;
  description: string;
  embed_model: string;
  embed_dim: number;
  created_at: string;
  updated_at: string;
}

export interface Document {
  id: string;
  corpus_id: string;
  title: string;
  source: string;
  content_hash: string;
  status: string;
  error_message?: string;
  created_at: string;
}

export interface RagHit {
  chunk_id: string;
  document_id: string;
  content: string;
  score: number;
  metadata: string;
}

export interface LLMCallLog {
  id: string;
  request_id: string;
  conversation_id?: string;
  provider: string;
  model: string;
  stream: boolean;
  status: string;
  prompt_tokens: number;
  completion_tokens: number;
  latency_ms: number;
  request_summary?: string;
  error_message?: string;
  created_at: string;
}

export interface RequestLog {
  id: string;
  request_id: string;
  method: string;
  path: string;
  path_template: string;
  status: number;
  latency_ms: number;
  request_body?: string;
  response_preview?: string;
  stream: boolean;
  conversation_id?: string;
  agent_run_id?: string;
  uid?: string;
  error_message?: string;
  created_at: string;
  llm_calls?: LLMCallLog[];
}

export interface AgentRun {
  id: string;
  uid?: string;
  conversation_id?: string;
  input: string;
  output: string;
  model: string;
  status: string;
  max_steps: number;
  step_count: number;
  error_message?: string;
  prompt_tokens: number;
  completion_tokens: number;
  created_at: string;
  finished_at?: string;
}

export interface AgentStep {
  id: string;
  run_id: string;
  step_index: number;
  kind: string;
  tool_name?: string;
  input_json?: string;
  output_text?: string;
}

export interface IntentItem {
  media_account_id: string;
  media_account_id_in: string;
  Mobile: string;
  icon_amount: string | number;
  TransferTryBest: boolean;
  media_account_ids: string[];
  KeyWordType: number;
  KeyWordTypeStr: string;
  ForbiddenReason: string;
  AuthCode: string;
  CopyNumber: number;
  CopyTaskNo: string;
}

export interface AnalyzeResult {
  data?: Record<string, unknown>;
  file_name?: string;
  file_chars?: number;
  truncated?: boolean;
  cache_hit?: boolean;
  content_hash?: string;
  extract_backend?: string;
  content?: string;
  usage?: Usage;
}
