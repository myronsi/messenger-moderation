import { accountPath, packPath } from '@/shared/config/routes';
import CasesPage from './CasesPage';

export default function CasesRoute() {
  return <CasesPage accountHref={accountPath} packHref={packPath} />;
}
