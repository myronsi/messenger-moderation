import { useSearchParams } from 'react-router-dom';
import { accountPath } from '@/shared/config/routes';
import QueuePage from './QueuePage';
import { filterFromSearch, filterToSearch } from './model/filter';

export default function QueueRoute() {
  const [params, setParams] = useSearchParams();
  return (
    <QueuePage
      accountHref={accountPath}
      initialFilter={filterFromSearch(params.toString())}
      onFilterChange={(filter) => setParams(filterToSearch(filter), { replace: true })}
    />
  );
}
