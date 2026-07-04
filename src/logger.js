const { createLogger, format, transports } = require("winston");

const colors = {
    error: "\x1b[31m",
    warn: "\x1b[33m",
    info: "\x1b[36m",
    debug: "\x1b[35m",
    reset: "\x1b[0m"
};

const logger = createLogger({
    level: process.env.LOG_LEVEL || "info",
    format: format.combine(
        format.timestamp({
            format: "DD/MM/YYYY HH:mm:ss"
        }),
        format.errors({ stack: true }),
        format.printf(({ timestamp, level, message, stack }) => {
            const color = colors[level] || "";
            const output = stack || message;

            return `${color}[${timestamp}] [${level.toUpperCase()}]${colors.reset} ${output}`;
        })
    ),
    transports: [
        new transports.Console()
    ]
});

logger.stream = {
    write(message) {
        logger.info(message.trim());
    }
};

module.exports = logger;