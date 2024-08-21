import fcntl
import json
import logging
import os
import traceback
from datetime import datetime


class CustomFormatter(logging.Formatter):
    def format(self, record: logging.LogRecord) -> str:
        source = ":".join([record.filename, str(record.lineno)])
        if isinstance(record.msg, dict):
            if "_source" in record.msg:  # override call stack
                source = record.msg["_source"]
                message = {k: v for k, v in record.msg.items() if k not in ["_source"]}
            else:
                message = record.msg
        else:
            message = record.getMessage()
        log = {
            'name': record.name,
            'level': record.levelname,
            'source': source,
            'create_time': datetime.fromtimestamp(record.created).strftime('%Y-%m-%d %H:%M:%S.%f'),
            'message': message
        }
        if record.exc_info:
            log["traceback"] = self.formatException(record.exc_info)
        str_log = json.dumps(log, ensure_ascii=False, separators=(',', ':'))
        if (length := len(str_log)) > 131072:
            log = {
                "error": "logging entity too long",
                "length": length,
                "traceback": ''.join(traceback.format_list(traceback.extract_stack()))
            }
            str_log = json.dumps(log)
        return str_log


class CustomStreamHandler(logging.StreamHandler):
    def __init__(self):
        super().__init__()
        self.setFormatter(CustomFormatter())


class CustomFileHandler(logging.Handler):
    terminator = '\n'

    def __init__(self, log_dir: str):
        super().__init__()
        self.log_dir: str = log_dir
        self.setFormatter(CustomFormatter())

    def emit(self, record: logging.LogRecord) -> None:
        str_log = self.format(record)
        date = datetime.fromtimestamp(record.created)
        filename = date.strftime('%Y%m%d%H.log')
        with open(os.path.join(self.log_dir, filename), 'a', encoding='utf-8') as f:
            fcntl.flock(f, fcntl.LOCK_EX)
            try:
                f.write(str_log + self.terminator)
            finally:
                fcntl.flock(f, fcntl.LOCK_UN)


LOG_DIR = 'logs'

if not os.path.exists(LOG_DIR):
    os.mkdir(LOG_DIR)

logger: logging.Logger = logging.getLogger('app')
logger.setLevel(logging.INFO)
logger.addHandler(CustomStreamHandler())
logger.addHandler(CustomFileHandler(LOG_DIR))


def get_logger(name: str = None):
    if name:
        return logger.getChild(name)
    return logger
