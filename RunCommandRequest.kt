fun timeout(millis: Long?): Builder {
    this.timeout = millis?.toDuration(kotlin.time.DurationUnit.MILLISECONDS)
    return this
}

fun timeout(duration: java.time.Duration?): Builder {
    this.timeout = duration?.toKotlinDuration()
    return this
}