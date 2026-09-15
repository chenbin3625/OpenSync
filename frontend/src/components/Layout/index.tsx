import React, { useState } from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import {
  Home, Server, Bell, Settings, User, LogOut, ChevronDown, Menu as MenuIcon, X
} from 'lucide-react';
import { useStore } from '../../stores/useStore';
import { logout } from '../../api/user';
import {
  DropdownMenu as Dropdown,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '../ui/dropdown-menu';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '../ui/alert-dialog';
import { Button } from '../ui/button';
import { cn } from '../../lib/utils';
import { icon } from '../../lib/styles';

const navItems = [
  { path: '/home', label: '任务管理', icon: Home },
  { path: '/engine', label: '引擎管理', icon: Server },
  { path: '/notify', label: '通知配置', icon: Bell },
  { path: '/setting', label: '系统设置', icon: Settings },
];

/** 导航项样式。桌面端为横向紧凑排布，移动端为整行排布，激活态保持一致 */
function navItemClass(isActive: boolean, mobile = false) {
  return cn(
    'flex items-center text-sm font-medium rounded-md transition-colors',
    mobile ? 'w-full gap-3 px-3 py-2' : 'gap-2 px-3 py-1.5',
    isActive
      ? 'bg-line text-teal-800 font-semibold hover:bg-line'
      : 'text-slate-600 hover:text-slate-900 hover:bg-line-soft'
  );
}

export default function Layout({ children }: { children: React.ReactNode }) {
  const navigate = useNavigate();
  const location = useLocation();
  const { setUserInfo, userInfo } = useStore();
  const [logoutDialogOpen, setLogoutDialogOpen] = useState(false);
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);

  const currentSection = '/' + (location.pathname.split('/')[1] || 'home');

  const handleLogout = async () => {
    try {
      await logout();
    } catch (err) {
      console.error('logout failed', err);
    }
    setUserInfo(null);
    setLogoutDialogOpen(false);
    navigate('/login');
  };

  // 窄屏保持文档流（保留移动端地址栏收起行为）；
  // 桌面端把外壳固定为一屏高，滚动交给内容区，页面本身不再出现整页滚动条。
  return (
    <div className="min-h-dvh flex flex-col md:h-dvh md:overflow-hidden">
      {/* 顶部导航栏 */}
      <header className="sticky top-0 z-40 h-14 border-b border-line bg-white/85 backdrop-blur-md px-4 sm:px-6 flex items-center justify-between shadow-xs">
        <div className="flex items-center gap-6">
          {/* 品牌标识 */}
          <div
            className="flex items-center gap-2.5 cursor-pointer select-none group"
            onClick={() => navigate('/home')}
            role="button"
            tabIndex={0}
            onKeyDown={(e) => (e.key === 'Enter' || e.key === ' ') && navigate('/home')}
          >
            <img
              src="/favicon.svg"
              alt="OpenSync"
              className="h-7 w-7 transition-transform group-hover:scale-105"
            />
            <span className="font-semibold text-slate-900 tracking-tight text-base">
              OpenSync
            </span>
          </div>

          {/* 桌面端导航 */}
          <nav aria-label="主导航">
            <div className="hidden md:flex items-center gap-1">
              {navItems.map((item) => {
                const Icon = item.icon;
                const isActive = currentSection === item.path;
                return (
                  <button
                    key={item.path}
                    onClick={() => navigate(item.path)}
                    aria-current={isActive ? 'page' : undefined}
                    className={navItemClass(isActive)}
                  >
                    <Icon className={icon.md} />
                    <span>{item.label}</span>
                  </button>
                );
              })}
            </div>
          </nav>
        </div>

        {/* 右侧用户区与操作 */}
        <div className="flex items-center gap-0 sm:gap-2">
          <div>
            <Dropdown>
              <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="sm"
                className="flex items-center gap-2 text-slate-600 hover:text-slate-900"
                aria-label="用户菜单"
              >
                <div className="h-6 w-6 rounded-full bg-teal-100 text-teal-800 flex items-center justify-center text-xs font-semibold">
                  <User className={icon.sm} />
                </div>
                <span className="hidden sm:inline max-w-[120px] truncate text-xs font-medium">
                  {userInfo?.userName || '用户'}
                </span>
                <ChevronDown className={cn(icon.sm, 'hidden sm:block opacity-50')} />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-40">
              <DropdownMenuItem
                onClick={() => setLogoutDialogOpen(true)}
                className="text-rose-600 focus:text-rose-700 focus:bg-rose-50 cursor-pointer"
              >
                <LogOut className={cn(icon.md, 'mr-2')} />
                <span>退出登录</span>
              </DropdownMenuItem>
            </DropdownMenuContent>
          </Dropdown>
          </div>

          {/* 移动端汉堡折叠按钮 */}
          <button
            type="button"
            onClick={() => setMobileMenuOpen((prev) => !prev)}
            className="md:hidden p-1.5 rounded-md text-slate-500 hover:text-slate-800 hover:bg-line-soft"
            aria-label="切换移动菜单"
          >
            {mobileMenuOpen ? <X className={icon.lg} /> : <MenuIcon className={icon.lg} />}
          </button>
        </div>
      </header>

      {/* 移动端下拉折叠菜单 */}
      {mobileMenuOpen && (
        <div className="md:hidden border-b border-line bg-white px-4 py-3 space-y-1 shadow-sm animate-in slide-in-from-top-2">
          {navItems.map((item) => {
            const Icon = item.icon;
            const isActive = currentSection === item.path;
            return (
              <button
                key={item.path}
                onClick={() => {
                  navigate(item.path);
                  setMobileMenuOpen(false);
                }}
                aria-current={isActive ? 'page' : undefined}
                className={navItemClass(isActive, true)}
              >
                <Icon className={icon.md} />
                <span>{item.label}</span>
              </button>
            );
          })}
        </div>
      )}

      {/* 主体内容：宽度铺满可视区（不再设 max-w 上限），纵向作为 flex 容器
          向下传递剩余高度，页面内部无需再按视口高度手算可用空间；
          桌面端由本区域承担滚动，页头因此始终可见 */}
      <main className="flex-1 min-h-0 w-full p-4 sm:p-6 flex flex-col md:overflow-y-auto">
        {children}
      </main>

      {/* 退出确认弹窗 */}
      <AlertDialog open={logoutDialogOpen} onOpenChange={setLogoutDialogOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>确认退出</AlertDialogTitle>
            <AlertDialogDescription>
              确定要退出当前 OpenSync 登录状态吗？
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction variant="destructive" onClick={handleLogout}>
              退出登录
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
