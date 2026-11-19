export class ApiError extends Error {
  constructor(public status: number, public code: string, message: string) {
    super(message);
  }
}

export type Role = 'admin' | 'moderator';

export interface Staff {
  id: number;
  username: string;
  role: Role;
  confidentiality_signed_at?: string;
}

export interface Meta {
  environment: 'development' | 'staging' | 'production';
  version: string;
}

const request = async <T>(method: string, path: string, body?: unknown): Promise<T> => {
  const response = await fetch(path, {
    method,
    credentials: 'same-origin',
    headers: body === undefined ? { Accept: 'application/json' } : { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (response.status === 204) return undefined as T;
  const data = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new ApiError(response.status, data.error ?? 'unknown', data.message ?? 'Something went wrong. Try again.');
  }
  return data as T;
};

export const api = {
  get: <T>(path: string) => request<T>('GET', path),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, body ?? {}),
  del: <T>(path: string, body?: unknown) => request<T>('DELETE', path, body ?? {}),
};

export const signIn = (username: string, password: string, code: string) =>
  api.post<{ staff: Staff; expires_at: string }>('/api/auth/login', { username, password, code });

export const signOut = () => api.post<void>('/api/auth/logout');

export const fetchMe = () => api.get<{ staff: Staff }>('/api/me');

export const fetchMeta = () => api.get<Meta>('/api/meta');

export type CheckState = 'ok' | 'slow' | 'failed';

export interface ComponentState {
  component: string;
  state: CheckState;
  latency_ms: number;
  detail?: string;
  checked_at: string;
  uptime_30d?: number;
}

export interface Incident {
  id: number;
  component: string;
  kind: 'failure' | 'monitoring_gap';
  started_at: string;
  ended_at?: string;
  detail?: string;
}

export interface StatusOverview {
  components: ComponentState[];
  open_incidents: Incident[];
  recent_incidents: Incident[];
}

export const fetchStatus = () => api.get<StatusOverview>('/api/status');
