
import datetime
import time
from typing import Optional

class Sandbox:
    def __init__(self, ..., idle_timeout: Optional[datetime.timedelta] = None):
        self.idle_timeout = idle_timeout
        self.last_activity_time = time.time()
        ...

    def create(self, ...):
        if self.idle_timeout:
            self.monitor_idle_timeout()
        ...

    def monitor_idle_timeout(self):
        while True:
            time.sleep(1)
            if time.time() - self.last_activity_time > self.idle_timeout.total_seconds():
                self.kill()
                break

    def update_last_activity_time(self):
        self.last_activity_time = time.time()

    def execute_command(self, ...):
        self.update_last_activity_time()
        ...

    def establish_connection(self, ...):
        self.update_last_activity_time()
        ...
