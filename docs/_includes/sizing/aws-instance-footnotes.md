- For production deployments, we recommend using non-burstable instance types, such as `m5` or `c5`, instead of burstable instances, such as `t3`.
  Burstable instances can experience significant performance degradation once CPU credits are exhausted, leading to poor user experience under sustained load.
