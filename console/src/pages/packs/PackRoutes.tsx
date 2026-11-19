import { useParams } from 'react-router-dom';
import type { PackKind } from '@/shared/api/decisions';
import { accountPath, packPath, PACKS_PATH } from '@/shared/config/routes';
import { PackPage, PacksPage } from './PackPage';

export function PacksRoute() {
  return <PacksPage packHref={packPath} />;
}

export function PackRoute() {
  const { kind = '', id = '' } = useParams();
  if (kind !== 'sticker_pack' && kind !== 'emoji_pack') {
    return <p className="rounded-md border border-dashed p-8 text-center text-sm">Unknown pack kind.</p>;
  }
  return <PackPage key={`${kind}:${id}`} kind={kind as PackKind} id={id} accountHref={accountPath} packsHref={PACKS_PATH} />;
}
