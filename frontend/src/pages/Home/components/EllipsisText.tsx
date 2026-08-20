import { useLayoutEffect, useRef, useState, type ReactNode } from 'react';
import Tooltip from 'antd/es/tooltip';
import Typography from 'antd/es/typography';
import { displayText } from '../homeUtils';

const { Text } = Typography;

export default function EllipsisText({
  value,
  tooltip,
  type,
  className,
  maxWidth,
}: {
  value: string | number | null | undefined;
  tooltip?: ReactNode;
  type?: 'secondary' | 'danger';
  className?: string;
  maxWidth?: number;
}) {
  const text = displayText(value);
  const measureRef = useRef<HTMLSpanElement>(null);
  const [overflow, setOverflow] = useState(false);

  useLayoutEffect(() => {
    const el = measureRef.current;
    if (!el || text === '--') {
      setOverflow(false);
      return undefined;
    }

    const check = () => {
      setOverflow(el.scrollWidth > el.clientWidth + 1);
    };
    check();
    if (typeof ResizeObserver === 'undefined') {
      return undefined;
    }
    const observer = new ResizeObserver(check);
    observer.observe(el);
    return () => observer.disconnect();
  }, [text]);

  if (text === '--') {
    return <Text type="secondary" className={className}>--</Text>;
  }

  const node = (
    <Text
      ref={measureRef}
      type={type}
      ellipsis
      className={className}
      style={maxWidth ? { display: 'inline-block', maxWidth, verticalAlign: 'bottom' } : undefined}
    >
      {text}
    </Text>
  );

  if (!overflow) {
    return node;
  }
  return <Tooltip title={tooltip || text}>{node}</Tooltip>;
}
