import type React from 'react';
import { useState, useEffect } from 'react';
import { Layout as AntLayout, Menu, Button, Typography, Popconfirm, Space, theme } from 'antd';
import {
  HomeOutlined,
  CloudServerOutlined,
  BellOutlined,
  SettingOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  LogoutOutlined,
  BulbOutlined,
  BulbFilled,
} from '@ant-design/icons';
import { useNavigate, useLocation } from 'react-router-dom';
import { useStore } from '../../stores/useStore';
import { logout } from '../../api/user';

const { Header, Sider, Content } = AntLayout;
const { Text } = Typography;

const menuItems = [
  { key: '/home', icon: <HomeOutlined />, label: '任务管理' },
  { key: '/engine', icon: <CloudServerOutlined />, label: '引擎管理' },
  { key: '/notify', icon: <BellOutlined />, label: '通知配置' },
  { key: '/setting', icon: <SettingOutlined />, label: '系统设置' },
];

export default function Layout({ children }: { children: React.ReactNode }) {
  const [collapsed, setCollapsed] = useState(false);
  const navigate = useNavigate();
  const location = useLocation();
  const { theme: themeMode, setTheme, setUserInfo, leftIndex, setLeftIndex } = useStore();
  const { token } = theme.useToken();

  const selectedKey = '/' + location.pathname.split('/')[1];

  useEffect(() => {
    setLeftIndex(selectedKey);
  }, [selectedKey, setLeftIndex]);

  const handleMenuClick = (e: { key: string }) => {
    navigate(e.key);
  };

  const handleLogout = async () => {
    try {
      await logout();
    } catch { /* ignore */ }
    setUserInfo(null);
    navigate('/login');
  };

  const toggleTheme = () => {
    setTheme(themeMode === 'dark' ? 'light' : 'dark');
  };

  return (
    <AntLayout className="app-shell" style={{ minHeight: '100vh' }}>
      <Sider
        className="app-sider"
        trigger={null}
        collapsible
        collapsed={collapsed}
        width={168}
        theme={themeMode}
        style={{
          background: token.colorBgContainer,
          borderRight: `1px solid ${token.colorBorderSecondary}`,
          overflow: 'hidden',
        }}
      >
        <div
          className="app-logo"
          style={{
            height: 56,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            gap: 8,
            background: token.colorBgContainer,
            borderBottom: `1px solid ${token.colorBorderSecondary}`,
            cursor: 'pointer',
          }}
          onClick={() => navigate('/home')}
        >
          <img className="app-logo-mark" src="/favicon.svg" alt="OpenSync" />
          {!collapsed && (
            <Text
              strong
              style={{
                color: token.colorText,
                whiteSpace: 'nowrap',
              }}
            >
              OpenSync
            </Text>
          )}
        </div>
        <Menu
          mode="inline"
          theme={themeMode}
          selectedKeys={[leftIndex || selectedKey]}
          items={menuItems}
          onClick={handleMenuClick}
          style={{
            background: token.colorBgContainer,
            border: 'none',
            color: token.colorText,
          }}
        />
      </Sider>
      <AntLayout>
        <Header
          className="app-header"
          style={{
            padding: '0 20px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            background: token.colorBgContainer,
            borderBottom: `1px solid ${token.colorBorderSecondary}`,
            height: 56,
            lineHeight: '56px',
          }}
        >
          <Button
            type="text"
            icon={collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
            onClick={() => setCollapsed(!collapsed)}
          />
          <Space>
            <Button
              type="text"
              icon={themeMode === 'dark' ? <BulbFilled /> : <BulbOutlined />}
              onClick={toggleTheme}
              title={themeMode === 'dark' ? '切换到浅色模式' : '切换到深色模式'}
            />
            <Popconfirm
              title="确认退出"
              description="确定要退出登录吗？"
              onConfirm={handleLogout}
              okText="确定"
              cancelText="取消"
            >
              <Button
                type="text"
                icon={<LogoutOutlined />}
              >
                {!collapsed && '登出'}
              </Button>
            </Popconfirm>
          </Space>
        </Header>
        <Content
          className="app-content"
          style={{
            margin: 20,
            padding: 20,
            overflow: 'auto',
            minHeight: 0,
          }}
        >
          {children}
        </Content>
      </AntLayout>
    </AntLayout>
  );
}
