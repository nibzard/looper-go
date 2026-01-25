# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| Latest  | ✅       |

## Reporting a Vulnerability

If you discover a security vulnerability, please report it responsibly.

### How to Report

1. **Do not** create a public issue
2. Send an email to: security@[relevant domain]
3. Include:
   - Description of the vulnerability
   - Steps to reproduce
   - Potential impact
   - Suggested fix (if known)

### What to Expect

- Acknowledgment within 48 hours
- Initial assessment within 7 days
- Coordination on disclosure timeline
- Credit in security advisory

## Plugin Security Model

Looper uses a plugin system that extends functionality through external binaries. This section explains the security model, threat model, and limitations of the current implementation.

### Overview

Plugins are external binaries that communicate with looper via JSON-RPC over stdin/stdout. Each plugin declares its capabilities in a manifest file, which looper uses to enforce security boundaries at runtime.

### Capability System

Plugins declare their required permissions via a `[capabilities]` section in their `looper-plugin.toml` manifest:

```toml
[capabilities]
can_modify_files = true     # Can create, read, and modify files
can_execute_commands = true # Can run shell commands
can_access_network = false  # Can make network requests
can_access_env = true       # Can read environment variables
```

#### Capability Types

| Capability | Description | Risk Level |
|------------|-------------|------------|
| `can_modify_files` | Plugin can create, modify, and delete files | **High** |
| `can_execute_commands` | Plugin can run arbitrary shell commands | **High** |
| `can_access_network` | Plugin can make HTTP/network requests | **Medium** |
| `can_access_env` | Plugin can read environment variables | **Medium** |

### Threat Model

#### Assumptions

1. **Trusted Plugin Source** - Users are expected to only install plugins from trusted sources
2. **Manifest Honesty** - The system assumes plugin manifests accurately declare capabilities
3. **No Binary Verification** - Plugin binaries are not cryptographically verified
4. **Same User Context** - Plugins run with the same user permissions as looper itself

#### Attack Vectors

**1. Malicious Plugin Manifest**
- A plugin could declare minimal capabilities but perform restricted operations anyway
- **Mitigation**: The capability system provides transparency only; it does not enforce restrictions at the OS level
- **Recommendation**: Only install plugins from trusted authors and review source code when possible

**2. Plugin Binary Compromise**
- If a plugin binary is replaced or tampered with, it could execute arbitrary code
- **Mitigation**: The system does not currently verify binary signatures
- **Recommendation**: Secure plugin directories with appropriate file permissions

**3. Command Injection**
- Plugins with `can_execute_commands = true` can run any shell command
- **Mitigation**: None - this is an intentional capability
- **Recommendation**: Carefully review which plugins have command execution enabled

**4. File System Access**
- Plugins with `can_modify_files = true` can write to any file accessible to the user
- **Mitigation**: None - this is an intentional capability
- **Recommendation**: Consider running looper in isolated environments for untrusted plugins

**5. Environment Variable Leakage**
- Plugins with `can_access_env = true` can read all environment variables
- **Mitigation**: None - this is an intentional capability
- **Recommendation**: Avoid storing secrets in environment variables when using untrusted plugins

### Sandboxing Limitations

**Important**: Looper does **NOT** provide strong sandboxing for plugins. The current implementation has the following limitations:

#### No OS-Level Isolation

- Plugins run as the same user as looper
- Plugins have access to the same filesystem as looper
- Plugins can access the same network as looper
- Plugins can read all environment variables (if `can_access_env = true`)

#### No Resource Limits

- Plugins can consume unlimited CPU and memory
- No timeout enforcement for plugin initialization
- Only JSON-RPC execution timeout is configurable

#### No Network Isolation

- Plugins with `can_access_network = true` can connect to any host/port
- No network filtering or rate limiting
- DNS queries are not monitored or restricted

#### No Seccomp/AppArmor Profiles

- No system call filtering
- No Linux security module integration
- Plugins can make any syscall available to the user

### Enforcement Mechanisms

The capability system provides the following enforcement:

1. **Manifest Declaration** - Plugins must declare capabilities in their manifest
2. **Runtime Checks** - Looper's `CapabilityManager` checks capabilities before operations
3. **Permission Levels** - Three levels: `granted`, `denied`, `prompt`
4. **Audit Logging** - Capability checks can be logged for audit trails

However, these checks **only apply within looper's code**. A malicious plugin binary can bypass these checks entirely by directly performing operations.

#### Restricted Execution Helpers

Looper provides helper types that enforce capability checks:

```go
// For command execution with capability checks
builder := plugin.NewRestrictedCommandBuilder(capManager, plugin)
cmd, err := builder.Command(ctx, "git", "status")

// For file operations with capability checks
writer := plugin.NewRestrictedFileWriter(capManager, plugin, baseDir)
err := writer.WriteFile(ctx, "output.txt", data, 0644)
```

These helpers only work if the plugin author uses them. A plugin that directly calls `os.WriteFile()` or `exec.Command()` bypasses these checks.

### Security Best Practices

#### For Users

1. **Principle of Least Privilege**
   - Only grant capabilities that a plugin actually needs
   - Review plugin manifests before installing
   - Use `looper plugin validate --verbose` to inspect capabilities

2. **Source Verification**
   - Only install plugins from trusted sources
   - Review plugin source code when available
   - Check plugin author reputation

3. **Isolation**
   - Consider running looper in a container or VM for untrusted plugins
   - Use separate user accounts for high-risk operations
   - Limit filesystem access with chroot or container bind mounts

4. **Audit**
   - Review plugin capability declarations regularly
   - Monitor log files for suspicious activity
   - Use `looper plugin info` to review installed plugins

5. **Environment Hygiene**
   - Avoid storing secrets in environment variables
   - Use credential managers instead of env vars
   - Rotate API keys regularly

#### For Plugin Authors

1. **Minimal Capabilities**
   - Only declare capabilities your plugin actually needs
   - Document why each capability is required
   - Consider alternative implementations that require fewer privileges

2. **Transparent Operation**
   - Log all sensitive operations clearly
   - Provide verbose mode showing what operations are being performed
   - Include security considerations in your plugin's README

3. **Defensive Programming**
   - Use looper's restricted execution helpers
   - Validate all user input
   - Handle errors gracefully without exposing sensitive information

4. **Secure Dependencies**
   - Keep dependencies updated
   - Use vendoring for reproducible builds
   - Review dependency security advisories

### Current Security Features

Looper-Go includes several security-conscious design choices:

- **Audit Logging** - JSONL logs track all plugin executions
- **No Remote Code Execution** - Plugins are local binaries only
- **Explicit Command Execution** - No shell interpolation in agent command construction
- **Configurable Workspace Boundaries** - Plugins can be configured with working directories
- **Manifest Validation** - Plugin manifests are validated before loading

### Future Security Enhancements

Planned security improvements (not yet implemented):

1. **Plugin Signing** - Cryptographic verification of plugin binaries
2. **Sandbox Mode** - Optional container-based isolation for plugins
3. **Resource Limits** - CPU, memory, and timeout enforcement
4. **Network Policies** - Configurable allow/deny lists for network access
5. **Seccomp Integration** - System call filtering on Linux
6. **Permission Prompting** - Interactive prompts for capability grants

### Reporting Plugin Security Issues

If you discover a security vulnerability in a plugin:

1. **For Built-in Plugins** (claude, codex, traditional)
   - Report via the main looper security channel
   - Include "Plugin Security:" in the subject line

2. **For Third-Party Plugins**
   - Report directly to the plugin author/maintainer
   - Follow the plugin's own security policy

3. **For the Capability System Itself**
   - Report via the main looper security channel
   - Include details about how the capability enforcement could be bypassed

## Dependency Security

We use GitHub Dependabot to automatically monitor and update dependencies. See `.github/dependabot.yml` for configuration.

Run `govulncheck ./...` to check for known vulnerabilities in dependencies.

## General Security Best Practices

- Keep dependencies updated
- Review changes before pulling
- Use `go vet` and static analysis
- Follow principle of least privilege
- Never commit secrets or credentials
- Run security audits regularly
- Use two-factor authentication on accounts
- Keep systems updated with security patches
