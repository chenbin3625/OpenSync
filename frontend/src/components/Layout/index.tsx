import type React from 'react';
import {
  App as AntApp,
  Button, Dropdown, Layout as AntLayout, Menu, Space, Typography,
} from 'antd';
import {
  BellOutlined, CloudServerOutlined, DownOutlined, HomeOutlined,
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
  const { setUserInfo, userInfo } = useStore();
  const { modal } = AntApp.useApp();
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

  const confirmLogout = () => {
    modal.confirm({
      title: '确认退出',
      content: '确定要退出登录吗？',
      okText: '确定',
      cancelText: '取消',
      onOk: handleLogout,
    });
  };

  const userMenuItems = [
    {
      key: 'logout',
      icon: <LogoutOutlined />,
      label: '退出登录',
    },
  ];

  const handleUserMenuClick = ({ key }: { key: string }) => {
    if (key === 'logout') {
      confirmLogout();
    }
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
          selectedKeys={[selectedKey]}
          items={menuItems}
          onClick={handleMenuClick}
        />
        <Space className="app-actions" size={8}>
          <Dropdown
            menu={{ items: userMenuItems, onClick: handleUserMenuClick }}
            trigger={['click']}
            placement="bottomRight"
          >
            <Button type="text" className="app-user-menu" aria-label="用户菜单">
              <UserOutlined />
              <span className="app-user-menu-label">{userInfo?.userName || '用户'}</span>
              <DownOutlined className="app-user-menu-arrow" />
            </Button>
          </Dropdown>
        </Space>
      </Header>
      <Content className="app-content">
        {children}
      </Content>
    </AntLayout>
  );
}
