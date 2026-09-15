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

const navItems = [
  { path: '/home', label: '任务管理', icon: Home },
  { path: '/engine', label: '引擎管理', icon: Server },
  { path: '/notify', label: '通知配置', icon: Bell },
  { path: '/setting', label: '系统设置', icon: Settings },
];

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

  return (
    <div className="min-h-screen bg-slate-50/80 text-slate-800 flex flex-col">
      {/* 顶部导航栏 */}
      <header className="sticky top-0 z-40 h-14 border-b border-slate-200/80 bg-white/85 backdrop-blur-md px-4 sm:px-6 flex items-center justify-between shadow-xs">
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
                    className={cn(
                      'flex items-center gap-2 px-3 py-1.5 text-sm font-medium rounded-md transition-colors',
                      isActive
                        ? 'bg-teal-50 text-teal-800 font-semibold'
                        : 'text-slate-600 hover:text-slate-900 hover:bg-slate-100/70'
                    )}
                  >
                    <Icon className="h-4 w-4" />
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
                  <User className="h-3.5 w-3.5" />
                </div>
                <span className="hidden sm:inline max-w-[120px] truncate text-xs font-medium">
                  {userInfo?.userName || '用户'}
                </span>
                <ChevronDown className="hidden sm:block h-3.5 w-3.5 opacity-50" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-40">
              <DropdownMenuItem
                onClick={() => setLogoutDialogOpen(true)}
                className="text-red-600 focus:text-red-700 focus:bg-red-50 cursor-pointer"
              >
                <LogOut className="h-4 w-4 mr-2" />
                <span>退出登录</span>
              </DropdownMenuItem>
            </DropdownMenuContent>
          </Dropdown>
          </div>

          {/* 移动端汉堡折叠按钮 */}
          <button
            type="button"
            onClick={() => setMobileMenuOpen((prev) => !prev)}
            className="md:hidden p-1.5 rounded-md text-slate-500 hover:text-slate-800 hover:bg-slate-100"
            aria-label="切换移动菜单"
          >
            {mobileMenuOpen ? <X className="h-5 w-5" /> : <MenuIcon className="h-5 w-5" />}
          </button>
        </div>
      </header>

      {/* 移动端下拉折叠菜单 */}
      {mobileMenuOpen && (
        <div className="md:hidden border-b border-slate-200 bg-white px-4 py-3 space-y-1 shadow-sm animate-in slide-in-from-top-2">
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
                className={cn(
                  'flex w-full items-center gap-3 px-3 py-2 text-sm font-medium rounded-md transition-colors',
                  isActive
                    ? 'bg-teal-50 text-teal-800 font-semibold'
                    : 'text-slate-600 hover:bg-slate-50'
                )}
              >
                <Icon className="h-4 w-4" />
                <span>{item.label}</span>
              </button>
            );
          })}
        </div>
      )}

      {/* 主体内容 */}
      <main className="flex-1 w-full max-w-7xl mx-auto p-4 sm:p-6">
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
