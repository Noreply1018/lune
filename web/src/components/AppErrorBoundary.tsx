import { Component, type ErrorInfo, type ReactNode } from "react";
import { Button } from "@/components/ui/button";

type Props = {
  children: ReactNode;
};

type State = {
  error: Error | null;
};

export default class AppErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("Lune admin boundary", error, info);
  }

  reset = () => {
    this.setState({ error: null });
  };

  render() {
    if (!this.state.error) {
      return this.props.children;
    }

    return (
      <div className="surface-section mx-auto flex min-h-[24rem] max-w-3xl flex-col items-center justify-center gap-5 px-6 py-10 text-center">
        <p className="eyebrow-label">Render Error</p>
        <h2 className="font-editorial text-[2rem] font-semibold tracking-[-0.05em] text-moon-800">
          页面出错了
        </h2>
        <p className="max-w-xl text-sm leading-7 text-moon-500">
          {this.state.error.message || "未知渲染错误"}
        </p>
        <Button onClick={this.reset}>重试</Button>
      </div>
    );
  }
}
