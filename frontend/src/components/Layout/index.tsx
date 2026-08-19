import type React from 'react';
import {
  Button, Layout as AntLayout, Menu, Popconfirm, Space, Typography,
} from 'antd';
import {
  BellOutlined, BulbFilled, BulbOutlined, CloudServerOutlined, HomeOutlined,
  LogoutOutlined, SettingOutlined, UserOutlined,
} from '@ant-design/icons';
import { useNavigate, useLocation } from 'react-router-dom';
import { useStore } from '../../stores/useStore';
import { logout } from '../../api/user';

const { Header, Content } = AntLayout;
const { Text } = Typography;

const menuItems = [
  { key: '/home', icon: <HomeOutlined />, label: <span className="app-nav-label">任务管理</span>, title: '任务管理' },
  { key: '/engine', icon: <CloudServerOutlined />, label: <span className="app-nav-label">引擎管理</span>, title: '引擎管理' },
  { key: '/notify', icon: <BellOutlined />, label: <span className="app-nav-label">通知配置</span>, title: '通知配置' },
  { key: '/setting', icon: <SettingOutlined />, label: <span className="app-nav-label">系统设置</span>, title: '系统设置' },
];

export default function Layout({ children }: { children: React.ReactNode }) {
  const navigate = useNavigate();
  const location = useLocation();
  const { theme: themeMode, setTheme, setUserInfo, userInfo } = useStore();
  const selectedKey = '/' + location.pathname.split('/')[1];

  const handleMenuClick = (e: { key: string }) => {
    navigate(e.key);
  };

  const handleLogout = async () => {
    try {
      await logout();
    } catch (err) {
      console.error('logout failed', err);
    }
    setUserInfo(null);
    navigate('/login');
  };

  const toggleTheme = () => {
    setTheme(themeMode === 'dark' ? 'light' : 'dark');
  };

  return (
    <AntLayout className="app-shell">
      <Header className="app-header">
        <div
          className="app-brand"
          onClick={() => navigate('/home')}
          role="button"
          tabIndex={0}
          onKeyDown={(event) => {
            if (event.key === 'Enter' || event.key === ' ') navigate('/home');
          }}
        >
          <img className="app-logo-mark" src="/favicon.svg" alt="OpenSync" />
          <Text strong>OpenSync</Text>
        </div>
        <Menu
          className="app-top-nav"
          mode="horizontal"
          theme={themeMode}
          selectedKeys={[selectedKey]}
          items={menuItems}
          onClick={handleMenuClick}
        />
        <Space className="app-actions" size={8}>
          {userInfo?.userName && (
            <span className="app-user-badge">
              <UserOutlined />
              <span>{userInfo.userName}</span>
            </span>
          )}
          <Button
            type="text"
            icon={themeMode === 'dark' ? <BulbFilled /> : <BulbOutlined />}
            onClick={toggleTheme}
            title={themeMode === 'dark' ? '切换到浅色模式' : '切换到深色模式'}
            aria-label={themeMode === 'dark' ? '切换到浅色模式' : '切换到深色模式'}
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
              登出
            </Button>
          </Popconfirm>
        </Space>
      </Header>
      <Content className="app-content">
        {children}
      </Content>
    </AntLayout>
  );
}
