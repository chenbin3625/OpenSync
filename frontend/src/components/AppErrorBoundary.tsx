import { Component, type ErrorInfo, type ReactNode } from 'react';

type Props = { children: ReactNode };
type State = { hasError: boolean };

export default class AppErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false };

  static getDerivedStateFromError(): State {
    return { hasError: true };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('unhandled application render error', error, info);
  }

  render() {
    if (!this.state.hasError) return this.props.children;
    return (
      <main style={{ padding: 32, textAlign: 'center' }}>
        <h2>页面加载失败</h2>
        <p>请刷新页面后重试。如果问题持续存在，请检查服务端日志。</p>
        <button type="button" onClick={() => window.location.reload()}>刷新页面</button>
      </main>
    );
  }
}
