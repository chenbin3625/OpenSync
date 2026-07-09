import type { ReactNode } from 'react';
import { Typography, Tooltip } from 'antd';
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
  if (text === '--') return <Text type="secondary" className={className}>--</Text>;
  return (
    <Tooltip title={tooltip || text}>
      <Text
        type={type}
        ellipsis
        className={className}
        style={maxWidth ? { display: 'inline-block', maxWidth, verticalAlign: 'bottom' } : undefined}
      >
        {text}
      </Text>
    </Tooltip>
  );
}
