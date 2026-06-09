# Realtime Task List Refresh Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor the realtime progress task list so refresh behavior is isolated, stable, and easier to maintain while preserving the existing UI.

**Architecture:** Keep backend contracts unchanged. Move realtime summary polling and realtime tab item polling into local React hooks, keep pure row normalization and pagination decisions in `taskRows.ts`, and leave `TaskList.tsx` as page orchestration plus history rendering.

**Tech Stack:** React 19, TypeScript, Ant Design, Node `node --test`, Vite build.

---

## File Structure

- Modify: `frontend/src/pages/Home/taskRows.ts`
  - Owns pure helpers for task identity, current response normalization, tab item response normalization, stable sorting, list merging, and page reset decisions.
- Modify: `frontend/tests/taskListRows.test.ts`
  - Covers the new pure helpers before production implementation.
- Create: `frontend/src/pages/Home/useRealtimeTask.ts`
  - Owns realtime summary polling, progress metric calculation, request ordering, and the last visible snapshot.
- Create: `frontend/src/pages/Home/useRealtimeTaskItems.ts`
  - Owns realtime tab item loading, active tab/page state, request ordering, clearing rules, and derived paged rows.
- Modify: `frontend/src/pages/Home/TaskList.tsx`
  - Removes realtime polling internals and consumes the two hooks. Keeps history polling and task actions.
- Test: `frontend/tests/taskListRows.test.ts`
  - Runs with `npm test -- tests/taskListRows.test.ts`.

## Task 1: Add Pure Refresh Helpers

**Files:**
- Modify: `frontend/tests/taskListRows.test.ts`
- Modify: `frontend/src/pages/Home/taskRows.ts`

- [ ] **Step 1: Write failing tests for normalization and list replacement**

Add tests that import `normalizeTaskItemPage`, `sortTaskItemsByCreateTimeDesc`, `getRealtimeTaskIdentity`, and `shouldReplaceRealtimeRows`. Example assertions:

```ts
test('normalizeTaskItemPage accepts paged, array, and null realtime item payloads', () => {
  assert.deepEqual(normalizeTaskItemPage({ dataList: [{ id: 1, status: 2 }], count: 9 }), {
    rows: [{ id: 1, status: 2 }],
    total: 9,
  });
  assert.deepEqual(normalizeTaskItemPage([{ id: 2, status: 7 }]), {
    rows: [{ id: 2, status: 7 }],
    total: 1,
  });
  assert.deepEqual(normalizeTaskItemPage(null), { rows: [], total: 0 });
});

test('shouldReplaceRealtimeRows replaces rows only when task, tab, or page changes', () => {
  assert.equal(shouldReplaceRealtimeRows(null, { status: 2, taskIdentity: '10:100', page: 1 }), true);
  assert.equal(shouldReplaceRealtimeRows({ status: 2, taskIdentity: '10:100', page: 1 }, { status: 2, taskIdentity: '10:100', page: 1 }), false);
  assert.equal(shouldReplaceRealtimeRows({ status: 2, taskIdentity: '10:100', page: 1 }, { status: 2, taskIdentity: '10:100', page: 2 }), true);
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm test -- tests/taskListRows.test.ts`

Expected: FAIL because the new exports do not exist.

- [ ] **Step 3: Implement pure helpers**

Add these exports to `taskRows.ts`:

```ts
export type RealtimeTaskLoadKey = {
  status: number;
  taskIdentity: string;
  page: number;
};

export function getRealtimeTaskIdentity(task: CurrentTaskIdentity & { taskId?: number | null }): string {
  return `${Number(task?.taskId || 0)}:${Number(task?.createTime || 0)}`;
}

export function normalizeTaskItemPage(data: PageData<TaskItem> | TaskItem[] | null): { rows: TaskItem[]; total: number } {
  if (!data) return { rows: [], total: 0 };
  if (Array.isArray(data)) return { rows: data, total: data.length };
  if ('dataList' in data && Array.isArray(data.dataList)) {
    return { rows: data.dataList, total: Number(data.count || 0) };
  }
  return { rows: [], total: 0 };
}

export function sortTaskItemsByCreateTimeDesc(rows: TaskItem[]): TaskItem[] {
  return [...rows].sort((a, b) => {
    const left = Number(a.createTime || 0);
    const right = Number(b.createTime || 0);
    if (left === right) return Number(b.id || 0) - Number(a.id || 0);
    return right - left;
  });
}

export function shouldReplaceRealtimeRows(previous: RealtimeTaskLoadKey | null, next: RealtimeTaskLoadKey): boolean {
  return !previous ||
    previous.status !== next.status ||
    previous.taskIdentity !== next.taskIdentity ||
    previous.page !== next.page;
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `npm test -- tests/taskListRows.test.ts`

Expected: PASS.

## Task 2: Extract Realtime Summary Hook

**Files:**
- Create: `frontend/src/pages/Home/useRealtimeTask.ts`
- Modify: `frontend/src/pages/Home/TaskList.tsx`

- [ ] **Step 1: Move current task polling into a hook**

Create a hook with this public API:

```ts
export function useRealtimeTask(jobId: string, enabled: boolean): {
  currentTask: CurrentTaskView | null;
  nowTick: number;
  refreshCurrentTask: () => Promise<void>;
};
```

The hook calls `jobGetTaskCurrent({ id: jobId })`, accepts only payloads with `doingTask`, calculates `remainSize`, `doneSize`, `speed`, `speedAvg`, and `remainTime`, polls every 3 seconds when enabled, and keeps the last visible snapshot on transient request errors.

- [ ] **Step 2: Replace TaskList current polling state**

In `TaskList.tsx`, remove `currentTask`, `prevTaskRef`, `nowTick`, `calcProgress`, and `fetchCurrent` local state. Consume `useRealtimeTask(jobId, showRealtime)` instead. Keep `handleTaskAction` calling `refreshCurrentTask()` after task actions.

- [ ] **Step 3: Run frontend tests**

Run: `npm test`

Expected: PASS.

## Task 3: Extract Realtime Tab Items Hook

**Files:**
- Create: `frontend/src/pages/Home/useRealtimeTaskItems.ts`
- Modify: `frontend/src/pages/Home/TaskList.tsx`
- Modify: `frontend/src/pages/Home/taskRows.ts`

- [ ] **Step 1: Move tab item state and refresh logic into a hook**

Create a hook with this public API:

```ts
export function useRealtimeTaskItems(params: {
  jobId: string;
  enabled: boolean;
  currentTask: CurrentTaskView | null;
  pageSize: number;
}): {
  activeTab: number;
  setActiveTab: (status: number) => void;
  tabTaskList: TaskItem[];
  pagedTabTaskList: TaskItem[];
  tabTaskTotal: number;
  tabTaskPage: number;
  setTabTaskPage: (page: number) => void;
  tabLoading: boolean;
};
```

The hook uses `doingTask` directly for status `1`, requests `jobGetTaskCurrent` for other statuses, clears rows when task identity or tab changes, replaces rows when page changes, merges rows on same-page polling, and ignores stale responses.

- [ ] **Step 2: Replace TaskList tab item state**

In `TaskList.tsx`, remove `activeTabRef`, `tabRequestRef`, `lastLoadedTabRef`, `prevTabTaskPageRef`, `fetchTabTasks`, `handleTabChange`, `pagedTabTaskList`, and tab pagination effects. Consume the new hook values and call `setActiveTab(Number(key))` in `Tabs`.

- [ ] **Step 3: Run focused tests**

Run: `npm test -- tests/taskListRows.test.ts`

Expected: PASS.

## Task 4: Split Realtime Presentation

**Files:**
- Modify: `frontend/src/pages/Home/TaskList.tsx`

- [ ] **Step 1: Extract local presentation components**

Inside `TaskList.tsx`, extract pure local components:

```ts
function RealtimeTaskItems(...)
function RealtimeTaskCard(...)
```

`RealtimeTaskItems` renders the `Spin`, empty state, file rows, and pagination. `RealtimeTaskCard` renders hero, scan section, metrics, pause action, and Tabs. Keep existing class names.

- [ ] **Step 2: Run lint and build**

Run: `npm run lint`

Expected: exit 0.

Run: `npm run build`

Expected: exit 0.

## Task 5: Final Verification

**Files:**
- Verify: all changed frontend files

- [ ] **Step 1: Run all frontend tests**

Run: `npm test`

Expected: PASS.

- [ ] **Step 2: Run frontend build**

Run: `npm run build`

Expected: PASS.

- [ ] **Step 3: Inspect git diff**

Run: `git diff -- frontend/src/pages/Home/TaskList.tsx frontend/src/pages/Home/taskRows.ts frontend/src/pages/Home/useRealtimeTask.ts frontend/src/pages/Home/useRealtimeTaskItems.ts frontend/tests/taskListRows.test.ts`

Expected: Diff only contains realtime task list refresh refactor and related tests.
