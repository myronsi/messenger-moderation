import { useParams } from 'react-router-dom';
import { accountPath, CONTENT_REPORTS_PATH, contentReportPath } from '@/shared/config/routes';
import { ContentReportPage } from './ContentReportPage';
import { ContentReportsPage } from './ContentReportsPage';

export function ContentReportsRoute() {
  return <ContentReportsPage reportHref={contentReportPath} />;
}

export function ContentReportRoute() {
  const { id = '' } = useParams();
  return <ContentReportPage key={id} id={id} accountHref={accountPath} reportHref={contentReportPath} listHref={CONTENT_REPORTS_PATH} />;
}
