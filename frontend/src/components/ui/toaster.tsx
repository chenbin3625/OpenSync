import React, { useState, useEffect } from 'react';
import { CheckCircle2, AlertCircle, Info, AlertTriangle, X } from 'lucide-react';
import { cn } from '../../lib/utils';

export type ToastType = 'success' | 'error' | 'info' | 'warning';

export interface ToastMessage {
  id: string;
  type: ToastType;
  content: React.ReactNode;
  duration?: number;
}

type ToastListener = (messages: ToastMessage[]) => void;

let listeners: ToastListener[] = [];
let toasts: ToastMessage[] = [];
const dismissTimers = new Map<string, ReturnType<typeof setTimeout>>();

function notify() {
  listeners.forEach((listener) => listener([...toasts]));
}

function scheduleDismiss(item: ToastMessage) {
  if (!item.duration || item.duration <= 0 || dismissTimers.has(item.id)) return;
  const timer = setTimeout(() => {
    dismissTimers.delete(item.id);
    toast.dismiss(item.id);
  }, item.duration);
  dismissTimers.set(item.id, timer);
}

function clearAllDismissTimers() {
  dismissTimers.forEach((timer) => clearTimeout(timer));
  dismissTimers.clear();
}

export const toast = {
  success(content: React.ReactNode, duration = 3000) {
    return toast.show('success', content, duration);
  },
  error(content: React.ReactNode, duration = 4000) {
    return toast.show('error', content, duration);
  },
  info(content: React.ReactNode, duration = 3000) {
    return toast.show('info', content, duration);
  },
  warning(content: React.ReactNode, duration = 3500) {
    return toast.show('warning', content, duration);
  },
  show(type: ToastType, content: React.ReactNode, duration = 3000) {
    const id = Math.random().toString(36).slice(2, 9);
    const item: ToastMessage = { id, type, content, duration };
    toasts = [...toasts, item];
    notify();
    scheduleDismiss(item);
    return id;
  },
  dismiss(id: string) {
    const timer = dismissTimers.get(id);
    if (timer !== undefined) {
      clearTimeout(timer);
      dismissTimers.delete(id);
    }
    toasts = toasts.filter((t) => t.id !== id);
    notify();
  },
};

export function Toaster() {
  const [items, setItems] = useState<ToastMessage[]>([]);

  useEffect(() => {
    listeners.push(setItems);
    toasts.forEach(scheduleDismiss);
    setItems([...toasts]);
    return () => {
      listeners = listeners.filter((l) => l !== setItems);
      if (listeners.length === 0) {
        clearAllDismissTimers();
      }
    };
  }, []);

  if (items.length === 0) return null;

  return (
    <div className="fixed top-4 right-4 left-4 sm:left-auto z-[9999] flex flex-col gap-2 pointer-events-none sm:w-full max-w-[calc(100vw-2rem)] sm:max-w-sm">
      {items.map((item) => {
        const Icon =
          item.type === 'success'
            ? CheckCircle2
            : item.type === 'error'
            ? AlertCircle
            : item.type === 'warning'
            ? AlertTriangle
            : Info;

        const borderClass =
          item.type === 'success'
            ? 'border-emerald-200 bg-surface text-emerald-950'
            : item.type === 'error'
            ? 'border-rose-200 bg-surface text-rose-950'
            : item.type === 'warning'
            ? 'border-amber-200 bg-surface text-amber-950'
            : 'border-line bg-surface text-slate-900';

        const iconColor =
          item.type === 'success'
            ? 'text-emerald-600'
            : item.type === 'error'
            ? 'text-rose-600'
            : item.type === 'warning'
            ? 'text-amber-600'
            : 'text-teal-600';

        return (
          <div
            key={item.id}
            className={cn(
              'pointer-events-auto flex items-start justify-between gap-3 px-4 py-3 rounded-lg border shadow-lg transition-all animate-in slide-in-from-top-3',
              borderClass
            )}
          >
            <div className="flex items-start gap-2.5 min-w-0 flex-1">
              <Icon className={cn('h-4 w-4 shrink-0 mt-0.5', iconColor)} />
              <div className="min-w-0 flex-1 text-sm font-medium leading-snug whitespace-pre-wrap break-words">
                {item.content}
              </div>
            </div>
            <button
              type="button"
              aria-label="关闭提示"
              onClick={() => toast.dismiss(item.id)}
              className="text-slate-400 hover:text-slate-600 rounded-sm p-0.5 shrink-0"
            >
              <X className="h-3.5 w-3.5" />
            </button>
          </div>
        );
      })}
    </div>
  );
}
