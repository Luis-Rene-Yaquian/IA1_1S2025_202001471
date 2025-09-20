# MediLogic — Informe de Casos Clínicos (Análisis y Coherencia)

## Metodología del sistema
MediLogic combina tres componentes lógicos:

1) **Afinidad por enfermedad (`afinidad/3`)**.  
   Cada enfermedad tiene una lista de síntomas requeridos. El sistema normaliza la severidad: *leve=1*, *moderado=2*, *severo=3*, y suma los pesos de los síntomas presentes que coinciden con la enfermedad. La afinidad es el porcentaje de esa suma respecto al máximo posible (nº de síntomas requeridos × 3). Esto favorece enfermedades cuyos síntomas clave estén presentes y con mayor severidad.

2) **Urgencia clínica (`urgencia/1`)**.  
   Hay banderas rojas (por ejemplo, *dolor_pecho* y *disnea≥moderado*) que elevan la urgencia a “Atención prioritaria”, incluso si la afinidad diagnóstica es baja o 0. Si hay síntomas severos o muchos síntomas, sube a “Consulta recomendada”.

3) **Medicamento seguro (`medicamento_seguro/2`)**.  
   Un fármaco se sugiere solo si  la trata de la enfermedad, y no está contraindicado por (b) alergias del paciente, (c) condiciones crónicas del paciente o (d) la propia enfermedad. Así se evita proponer fármacos potencialmente dañinos.

---

## Caso 1 — Gripe sin alergias
**Entrada**: fiebre (moderado), tos (leve), dolor_garganta (severo); alergias: ninguna; crónicas: ninguna.  
**Salida**: Gripe (**67%**), medicamento: **paracetamol**, urgencia: **Consulta recomendada**.  
**Coherencia :**  
- **Afinidad**: Los tres síntomas **clave** de gripe (fiebre, tos, dolor_garganta) están presentes. El puntaje es 2 (fiebre) + 1 (tos) + 3 (dolor_garganta) = **6**. El máximo para gripe son 3 síntomas × 3 = **9**. Afinidad = 6/9 ≈ **67%**. Esto refleja **consistencia sintomática** fuerte con gripe.  
- **Urgencia**: Hay al menos un síntoma **severo** (dolor_garganta), lo que eleva a **“Consulta recomendada”** (sin llegar a bandera roja).  
- **Medicamento**: `paracetamol` **trata** gripe y no está bloqueado por alergia, crónica ni por la propia enfermedad. En cambio, aunque `ibuprofeno` “trate” gripe, está **contraindicado para gripe** en la KB, por lo que no se sugiere.  
- **Observación de salida secundaria**: Es esperable que aparezca **Asma** con afinidad baja (~17%) si solo comparte *tos* como síntoma. Cálculo: Asma requiere (disnea, tos). Con solo *tos leve* presente, puntaje = 1; máximo = 2×3=6 → 1/6 ≈ **17%**. Este “arrastre” por síntoma compartido es **coherente** con el modelo porcentual.

---

## Caso 2 — Gripe con alergia a paracetamol
**Entrada**: mismos síntomas que Caso 1; alergias: **alergia_paracetamol**; crónicas: ninguna.  
**Salida**: Gripe (**67%**), medicamento: **N/A**, urgencia: **Consulta recomendada**.  
**Coherencia :**  
- **Afinidad**: Es la misma del Caso 1 (mismos síntomas/severidades) → **67%**.  
- **Urgencia**: Igual que Caso 1 → **“Consulta recomendada”**.  
- **Medicamento**: Aunque `paracetamol` trata gripe, ahora está **bloqueado por alergia**. Y `ibuprofeno` sigue **bloqueado por la enfermedad**. El sistema, correctamente, **no sugiere nada**. Es un comportamiendo deseable: **la seguridad del paciente prima** sobre “forzar” una sugerencia.

---

## Caso 3 — Reflujo con crónica prolongación QT
**Entrada**: pirosis (severo), regurgitacion (moderado); alergias: ninguna; crónicas: **prolongacion_qt**.  
**Salida**: Reflujo (**83%**), medicamento: **N/A**, urgencia: **Consulta recomendada**.  
**Coherencia :**  
- **Afinidad**: Reflujo requiere `pirosis` y `regurgitacion`. Puntaje = 3 + 2 = **5**; máximo = 2×3=**6** → 5/6 ≈ **83%**. Altísima consistencia con reflujo.  
- **Urgencia**: Hay un síntoma **severo** → **“Consulta recomendada”**.  
- **Medicamento**:  
  - `aines` está **bloqueado por la enfermedad reflujo**.  
  - `omeprazol`, aunque trata reflujo, está **contraindicado por la crónica** `prolongacion_qt`.  
  Resultado: **sin sugerencia**. Esto es **coherente** porque el sistema respeta simultáneamente **dos motivos de bloqueo distintos** (por enfermedad y por condición crónica).

---

## Caso 4 — Asma
**Entrada**: disnea (severo), tos (moderado); alergias: ninguna; crónicas: ninguna.  
**Salida**: Asma (**83%**), medicamento: **salbutamol**, urgencia: **Atención prioritaria**.  
**Coherencia :**  
- **Afinidad**: Asma requiere `disnea` y `tos`. Puntaje = 3 + 2 = **5**; máximo = 6 → **83%**. Encaja perfectamente con la clínica de asma.  
- **Urgencia**: `disnea` **severa** dispara **bandera roja** según las reglas (o, al menos, eleva la prioridad clínica), de modo que la salida correcta es **“Atención prioritaria”**.  
- **Medicamento**: `salbutamol` trata asma y **no** está contraindicado ni por enfermedad ni por alergias/crónicas del paciente. Por eso la propuesta es **coherente** y clínicamente razonable dentro del alcance educativo del modelo.

---

## Caso 5 — Dolor de pecho leve
**Entrada**: dolor_pecho (leve); alergias: ninguna; crónicas: ninguna.  
**Salida**: Afinidad **0%** para todas, urgencia: **Atención prioritaria**.  
**Coherencia:**  
- **Afinidad**: No hay coincidencia con los conjuntos de síntomas definidos para gripe, reflujo o asma, por lo que la afinidad justa es **0%**.  
- **Urgencia**: `dolor_pecho` es una **bandera roja** per se. Las reglas elevan la salida a **“Atención prioritaria”** aun con 0% de afinidad, porque lo correcto es **priorizar la acción clínica inmediata** sobre un etiquetado diagnóstico.  
- **Medicamentos**: Con afinidad 0% y bandera roja, **no corresponde** sugerir tratamiento etiológico desde el sistema; lo correcto es la **derivación urgente**. Si la interfaz muestra nombres de fármaco asociados a filas con 0%, se debe interpretar **solo como metadatos de la enfermedad** y **no como recomendación**. (Si se desea, podría filtrarse la sugerencia a “afinidad > 0 y con match real” para evitar confusión.)

---

## Conclusiones
- El sistema es coherente porque:  
  (1) las coincidencias sintomáticas con severidad producen afinidades que reflejan lo observado (gripe y asma en 67–83%, reflujo en 83%);  
  (2) las banderas rojas prevalecen sobre la afinidad (dolor_pecho / disnea severa → Atención prioritaria);  
  (3) la sugerencia farmacológica respeta los tres filtros de seguridad: alergias, crónicas y contraindicaciones por enfermedad.  
- Estos cinco casos fortalecen el entendimiento del modelo lógico: muestran el porcentaje de síntomas, la priorización clínica y las restricciones terapéuticas interactúan para producir salidas explicables y defendibles.  
