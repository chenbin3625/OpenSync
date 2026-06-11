import type { MessageInstance } from 'antd/es/message/interface';

// Holds the context-aware message instance from antd App component.
// Initialized by MessageInitializer mounted inside <AntApp>.
let messageInstance: MessageInstance | null = null;

export const setMessageInstance = (instance: MessageInstance) => {
  messageInstance = instance;
};

export const getMessageInstance = () => messageInstance;
