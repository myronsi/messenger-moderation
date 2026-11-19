import { useParams } from 'react-router-dom';
import { accountPath, QUEUE_PATH } from '@/shared/config/routes';
import AccountPage from './AccountPage';

export default function AccountRoute({ isAdmin = false }: { isAdmin?: boolean }) {
  const { id = '' } = useParams();
  return <AccountPage key={id} userId={id} accountHref={accountPath} queueHref={QUEUE_PATH} isAdmin={isAdmin} />;
}
