export function deadlineText(deadline: string, now: Date): { text: string; overdue: boolean; soon: boolean } {
  const ms = new Date(deadline).getTime() - now.getTime();
  const hours = Math.floor(Math.abs(ms) / 3_600_000);
  const minutes = Math.floor((Math.abs(ms) % 3_600_000) / 60_000);
  const span = hours > 0 ? `${hours} h ${minutes} min` : `${minutes} min`;
  if (ms < 0) return { text: `Overdue by ${span}`, overdue: true, soon: false };
  return { text: `${span} left`, overdue: false, soon: ms < 12 * 3_600_000 };
}
