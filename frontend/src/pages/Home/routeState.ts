export type HomeTabKey = 'overview' | 'realtime' | 'history';

const homeTabKeys = new Set<HomeTabKey>(['overview', 'realtime', 'history']);

export type HomeRouteState = {
  tab: HomeTabKey;
  jobId: number | null;
};

export function isHomeTabKey(value: string | null): value is HomeTabKey {
  return homeTabKeys.has(value as HomeTabKey);
}

export function readHomeRouteState(searchParams: URLSearchParams): HomeRouteState {
  const rawTab = searchParams.get('tab');
  const rawJobId = Number(searchParams.get('jobId'));

  return {
    tab: isHomeTabKey(rawTab) ? rawTab : 'overview',
    jobId: Number.isInteger(rawJobId) && rawJobId > 0 ? rawJobId : null,
  };
}

export function buildHomeRouteSearch(
  currentSearchParams: URLSearchParams,
  state: HomeRouteState,
): URLSearchParams {
  const nextSearchParams = new URLSearchParams(currentSearchParams);

  nextSearchParams.set('tab', state.tab);
  if (state.jobId) {
    nextSearchParams.set('jobId', String(state.jobId));
  } else {
    nextSearchParams.delete('jobId');
  }

  return nextSearchParams;
}
