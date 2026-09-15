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

function notify() {
  listeners.forEach((listener) => listener([...toasts]));
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

    if (duration > 0) {
      setTimeout(() => {
        toast.dismiss(id);
      }, duration);
    }
    return id;
  },
  dismiss(id: string) {
    toasts = toasts.filter((t) => t.id !== id);
    notify();
  },
};

export function Toaster() {
  const [items, setItems] = useState<ToastMessage[]>([]);

  useEffect(() => {
    listeners.push(setItems);
    setItems([...toasts]);
    return () => {
      listeners = listeners.filter((l) => l !== setItems);
    };
  }, []);

  if (items.length === 0) return null;

  return (
    <div className="fixed top-4 right-4 z-[9999] flex flex-col gap-2 pointer-events-none max-w-sm w-full">
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
            ? 'border-emerald-200 bg-white text-emerald-950'
            : item.type === 'error'
            ? 'border-rose-200 bg-white text-rose-950'
            : item.type === 'warning'
            ? 'border-amber-200 bg-white text-amber-950'
            : 'border-slate-200 bg-white text-slate-900';

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
              'pointer-events-auto flex items-center justify-between gap-3 px-4 py-3 rounded-lg border shadow-lg transition-all animate-in slide-in-from-top-3',
              borderClass
            )}
          >
            <div className="flex items-center gap-2.5 min-w-0">
              <Icon className={cn('h-4 w-4 shrink-0', iconColor)} />
              <div className="text-sm font-medium leading-tight truncate">{item.content}</div>
            </div>
            <button
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
