export const QUEUE_PATH = '/queue';

export const accountPath = (id: string) => `/users/${encodeURIComponent(id)}`;

export const CASES_PATH = '/cases';

export const PACKS_PATH = '/packs';

export const packPath = (kind: string, id: string) => `/packs/${encodeURIComponent(kind)}/${encodeURIComponent(id)}`;
