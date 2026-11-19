import { useParams } from 'react-router-dom';
import { accountPath, QUEUE_PATH } from '@/shared/config/routes';
import AccountPage from './AccountPage';

export default function AccountRoute() {
  const { id = '' } = useParams();
  return <AccountPage key={id} userId={id} accountHref={accountPath} queueHref={QUEUE_PATH} />;
}
