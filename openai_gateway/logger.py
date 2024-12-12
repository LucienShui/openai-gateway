import fcntl
import json
import logging
import os
import traceback
from datetime import datetime
from typing import Optional

from pydantic import BaseModel, Field


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

    def __init__(self, log_dir: str, pattern: str, keep_count: Optional[int] = None):
        super().__init__()
        self.setFormatter(CustomFormatter())

        self.log_dir: str = log_dir
        self.pattern: str = pattern
        self.keep_count: Optional[int] = keep_count
        self.current_filename: str = ""

    def get_file_date(self, filename: str) -> Optional[datetime]:
        if filename != self.current_filename and os.path.isfile(os.path.join(self.log_dir, filename)):
            try:
                return datetime.strptime(filename, self.pattern)
            except ValueError:
                pass
        return None

    def rollover(self, keep_count: int):
        rollover_candidates = []
        for filename in os.listdir(self.log_dir):
            if date := self.get_file_date(filename):
                rollover_candidates.append((filename, date))
        should_rollover = sorted(rollover_candidates, key=lambda pair: pair[1], reverse=True)[keep_count:]
        for filename, date in should_rollover:
            try:
                os.remove(os.path.join(self.log_dir, filename))
            except FileNotFoundError:
                pass

    def emit(self, record: logging.LogRecord) -> None:
        str_log = self.format(record)
        date = datetime.fromtimestamp(record.created)
        filename = date.strftime(self.pattern)
        if self.keep_count is not None and filename != self.current_filename:
            self.current_filename = filename
            self.rollover(self.keep_count)
        with open(os.path.join(self.log_dir, filename), 'a', encoding='utf-8') as f:
            fcntl.flock(f, fcntl.LOCK_EX)
            try:
                f.write(str_log + self.terminator)
            finally:
                fcntl.flock(f, fcntl.LOCK_UN)


class LoggingConfig(BaseModel):
    dir: str = Field(default="logs")
    pattern: str = Field(default="%Y%m%d.log.jsonl")
    keep_count: Optional[int] = Field(default=None)


logging_config = LoggingConfig.model_validate_json(os.getenv("LOGGING_CONFIG", "{}"))

if not os.path.exists(logging_config.dir):
    os.mkdir(logging_config.dir)

logger: logging.Logger = logging.getLogger('app')
logger.setLevel(logging.INFO)
logger.addHandler(CustomStreamHandler())
logger.addHandler(CustomFileHandler(logging_config.dir, logging_config.pattern, logging_config.keep_count))


def get_logger(name: str = None):
    if name:
        return logger.getChild(name)
    return logger
