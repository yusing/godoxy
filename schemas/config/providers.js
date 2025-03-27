export const includeExamples = ["file1.yml", "file2.yml"];
export const dockerExamples = [
    { local: "$DOCKER_HOST" },
    { remote: "tcp://10.0.2.1:2375" },
    { remote2: "ssh://root:1234@10.0.2.2" },
];
export const notificationExamples = [
    {
        name: "gotify",
        provider: "gotify",
        url: "https://gotify.domain.tld",
        token: "abcd",
    },
    {
        name: "discord",
        provider: "webhook",
        template: "discord",
        url: "https://discord.com/api/webhooks/1234/abcd",
    },
];
export const agentExamples = ["10.0.2.3:8890", "10.0.2.4:8890"];
