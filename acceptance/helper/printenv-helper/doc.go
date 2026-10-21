// Command printenv-helper is the child process in acceptance tests. It writes
// the environment variables named as arguments to child-env.txt in the
// working directory, one KEY=value line each, and never to stdout, so the
// tests can check that secretref itself prints no secret.
//
// Built under the name docker, it skips everything up to "compose", accepts
// and ignores -f, and can
// print the compose override secretref passed through COMPOSE_FILE.
package main
