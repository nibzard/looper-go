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

### Security Best Practices

- Keep dependencies updated
- Review changes before pulling
- Use `go vet` and static analysis
- Follow principle of least privilege
- Never commit secrets or credentials

## Security Features

Looper-Go includes several security-conscious design choices:

- Audit logging via JSONL files
- No remote code execution
- Explicit command execution only
- Configurable workspace boundaries

## Dependency Security

We use GitHub Dependabot to automatically monitor and update dependencies. See `.github/dependabot.yml` for configuration.
