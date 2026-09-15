import { toast } from '../components/ui/toaster';

export interface SimpleMessageInstance {
  success: (content: React.ReactNode) => void;
  error: (content: React.ReactNode) => void;
  info: (content: React.ReactNode) => void;
  warning: (content: React.ReactNode) => void;
}

let messageInstance: SimpleMessageInstance = {
  success: (msg) => toast.success(msg),
  error: (msg) => toast.error(msg),
  info: (msg) => toast.info(msg),
  warning: (msg) => toast.warning(msg),
};

export const setMessageInstance = (instance: SimpleMessageInstance) => {
  messageInstance = instance;
};

export const getMessageInstance = () => messageInstance;
