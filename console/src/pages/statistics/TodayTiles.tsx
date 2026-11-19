import type { StatsToday } from '@/shared/api/client';
import { formatValue } from './format';
import { numberOf, type TileDef } from './metrics';

interface TodayTilesProps {
  id: string;
  title: string;
  tiles: TileDef[];
  today: StatsToday;
}

const TIME = new Intl.DateTimeFormat('en-GB', { hour: '2-digit', minute: '2-digit', timeZone: 'UTC' });

const TodayTiles = ({ id, title, tiles, today }: TodayTilesProps) => (
  <section aria-labelledby={id} className="space-y-2">
    <div className="flex flex-wrap items-baseline gap-x-3">
      <h2 id={id} className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{title}</h2>
      <p className="text-xs text-muted-foreground">Today so far, until {TIME.format(new Date(today.as_of))} UTC, next to all of yesterday</p>
    </div>
    <dl className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
      {tiles.map((tile) => {
        const now = numberOf(today.today, tile.key);
        const before = numberOf(today.yesterday, tile.key);
        return (
          <div key={tile.key} className="rounded-lg border border-border bg-card px-3 py-2">
            <dt className="text-xs text-muted-foreground">{tile.label}</dt>
            <dd className="text-xl font-semibold tabular-nums text-foreground">{formatValue(now, tile.format)}</dd>
            <dd className="text-xs text-subtle-foreground">
              Yesterday <span className="tabular-nums">{formatValue(before, tile.format)}</span>
            </dd>
          </div>
        );
      })}
    </dl>
  </section>
);

export default TodayTiles;
