import type React from 'react';
import { useState, useEffect } from 'react';
import { Layout as AntLayout, Menu, Button, theme, Typography, Popconfirm } from 'antd';
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
  SyncOutlined,
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
    <AntLayout style={{ minHeight: '100vh' }}>
      <Sider
        trigger={null}
        collapsible
        collapsed={collapsed}
        width={220}
        style={{
          background: token.colorBgContainer,
          borderRight: `1px solid ${token.colorBorderSecondary}`,
          overflow: 'hidden',
        }}
      >
        <div
          style={{
            height: 56,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            gap: 8,
            borderBottom: `1px solid ${token.colorBorderSecondary}`,
            cursor: 'pointer',
          }}
          onClick={() => navigate('/home')}
        >
          <SyncOutlined
            style={{
              fontSize: collapsed ? 18 : 22,
              color: token.colorPrimary,
            }}
            spin={!collapsed}
          />
          {!collapsed && (
            <Text
              strong
              style={{
                fontSize: 17,
                color: token.colorPrimary,
                whiteSpace: 'nowrap',
              }}
            >
              OpenSync
            </Text>
          )}
        </div>
        <Menu
          mode="inline"
          selectedKeys={[leftIndex || selectedKey]}
          items={menuItems}
          onClick={handleMenuClick}
          style={{
            border: 'none',
            padding: '8px 0',
          }}
        />
      </Sider>
      <AntLayout>
        <Header
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
            style={{ fontSize: 16 }}
          />
          <div style={{ display: 'flex', gap: 4, alignItems: 'center' }}>
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
          </div>
        </Header>
        <Content
          style={{
            margin: 20,
            padding: 20,
            overflow: 'auto',
            borderRadius: token.borderRadiusLG,
            background: token.colorBgContainer,
            minHeight: 0,
          }}
        >
          {children}
        </Content>
      </AntLayout>
    </AntLayout>
  );
}
