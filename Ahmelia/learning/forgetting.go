package learning

/* =======================
   FORGETTING
   Punto de extensión: system/Learning.go original no tenía ninguna
   noción de "olvido" (nunca borra Knowledge, solo baja confianza en
   ReportFailure). Este archivo queda listo para cuando se decida
   implementar poda real de memoria (ej. eliminar Knowledge con
   confianza e importancia por debajo de un umbral durante mucho
   tiempo). No se agrega lógica inventada todavía porque memory.LongMemory
   no expone hoy un método de borrado — hay que decidir esa API primero.
======================= */
