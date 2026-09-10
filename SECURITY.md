# Security Policy

## Supported Versions

Security fixes are provided for the latest stable release. Please upgrade before reporting a problem that has already been fixed in a newer release.

## Reporting a Vulnerability

Please do not open a public issue for vulnerabilities, leaked tokens, authentication bypasses, or data exposure reports.

Send the report privately to the maintainer with:

- Affected version or Docker image tag.
- Deployment method: Docker Compose, Docker CLI, binary, or source build.
- Reproduction steps or proof of concept.
- Logs or screenshots with tokens, account names, private paths, and hostnames redacted.
- Whether the issue is already being exploited or only locally reproduced.

The maintainer will acknowledge the report, investigate, and publish a fix or mitigation when confirmed.

## Hardening Notes

- Never expose the OpenSync service directly to the public internet without an authenticated reverse proxy, firewall, or VPN.
- Never publish the mounted `data/` directory, `backend/data`, database files, config files, logs, or files containing AList / OpenList tokens.
- Keep `data/secret.key` private and persistent. It protects login cookies and encrypted sensitive data.
- Rotate AList / OpenList tokens immediately if runtime data was shared accidentally.
- Back up the mounted `data/` directory before upgrades.
- Prefer pinned image tags for production deployments when repeatability matters.

---

# 安全策略

## 支持版本

安全修复优先面向最新稳定版发布。报告问题前，建议先确认是否已经在新版中修复。

## 漏洞反馈

请不要用公开 Issue 反馈漏洞、令牌泄漏、认证绕过或数据暴露问题。

请私下联系维护者，并尽量提供：

- 受影响版本或 Docker 镜像标签。
- 部署方式：Docker Compose、Docker 命令、二进制或源码构建。
- 复现步骤或验证方式。
- 已脱敏的日志或截图，注意隐藏令牌、账号、私有路径和主机名。
- 问题是否已经被实际利用，还是只在本地复现。

维护者会确认反馈、排查问题，并在确认后发布修复或缓解方案。

## 加固建议

- 不要在没有认证反向代理、防火墙或 VPN 的情况下把 OpenSync 直接暴露到公网。
- 不要公开 Docker 挂载的 `data/` 目录、`backend/data`、数据库、配置文件、日志或任何包含 AList / OpenList Token 的文件。
- 妥善保存并保护 `data/secret.key`，它会影响登录 Cookie 和敏感数据加解密。
- 如果运行数据目录被误分享，请立即更换 AList / OpenList Token。
- 升级前先备份挂载的 `data/` 目录。
- 生产部署建议使用固定版本镜像标签，方便回滚和复现。
