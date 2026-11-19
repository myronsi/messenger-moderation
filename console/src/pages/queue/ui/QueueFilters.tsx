import { AGE_LABELS, REASON_LABELS, STATUS_LABELS } from '@/entities/moderation';
import { ACCOUNT_STATUSES, QUEUE_AGES, REPORT_REASONS, type QueueFilter } from '@/shared/api/review';
import { Button } from '@/shared/ui/button';
import { Select } from '@/shared/ui/select';
import { DEFAULT_FILTER, isDefaultFilter } from '../model/filter';

const STATUS_OPTIONS = [
  { value: 'any' as const, label: 'All statuses' },
  ...ACCOUNT_STATUSES.map((status) => ({ value: status, label: STATUS_LABELS[status] })),
];

const REASON_OPTIONS = [
  { value: 'any' as const, label: 'All reasons' },
  ...REPORT_REASONS.map((reason) => ({ value: reason, label: REASON_LABELS[reason] })),
];

const AGE_OPTIONS = QUEUE_AGES.map((age) => ({ value: age, label: AGE_LABELS[age] }));

interface QueueFiltersProps {
  value: QueueFilter;
  onChange: (value: QueueFilter) => void;
}

export function QueueFilters({ value, onChange }: QueueFiltersProps) {
  return (
    <div role="group" aria-label="Filters" className="flex flex-wrap items-end gap-3">
      <Select
        label="Status"
        className="w-full sm:w-44"
        value={value.status}
        options={STATUS_OPTIONS}
        onValueChange={(status) => onChange({ ...value, status })}
      />
      <Select
        label="Reason"
        className="w-full sm:w-52"
        value={value.reason}
        options={REASON_OPTIONS}
        onValueChange={(reason) => onChange({ ...value, reason })}
      />
      <Select
        label="Age"
        className="w-full sm:w-44"
        value={value.age}
        options={AGE_OPTIONS}
        onValueChange={(age) => onChange({ ...value, age })}
      />
      {!isDefaultFilter(value) && (
        <Button variant="ghost" size="sm" onClick={() => onChange(DEFAULT_FILTER)}>
          Clear filters
        </Button>
      )}
    </div>
  );
}
