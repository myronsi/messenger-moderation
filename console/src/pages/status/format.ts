export const formatDuration = (from: string, to?: string, now = Date.now()) => {
  const minutes = Math.max(0, Math.round(((to ? Date.parse(to) : now) - Date.parse(from)) / 60000));
  if (minutes < 60) return `${minutes} min`;
  const hours = Math.floor(minutes / 60);
  if (hours < 48) return `${hours} h ${minutes % 60} min`;
  return `${Math.floor(hours / 24)} d ${hours % 24} h`;
};

export const formatTime = (value: string) =>
  new Date(value).toLocaleString('en-GB', { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit' });

export const formatUptime = (ratio?: number) => (ratio === undefined ? '-' : `${(ratio * 100).toFixed(ratio >= 0.9995 ? 1 : 2)} %`);
