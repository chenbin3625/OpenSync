type PollingDocument = {
  hidden?: boolean;
} | null | undefined;

export function canPollVisibleDocument(documentLike?: PollingDocument): boolean {
  return documentLike?.hidden !== true;
}

export function canPollCurrentDocument(): boolean {
  if (typeof document === 'undefined') return true;
  return canPollVisibleDocument(document);
}
