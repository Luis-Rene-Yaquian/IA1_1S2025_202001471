% ==== KB generada por RPA ====
sintoma(regurgitacion).
sintoma(fiebre).
sintoma(tos).
sintoma(dolor_garganta).
sintoma(pirosis).

enfermedad(gripe, "Gripe", respiratorio, viral).
enfermedad(reflujo, "Enfermedad por reflujo gastroesofágico", digestivo, cronico).
descripcion_enf(gripe, "Infección respiratoria alta.").
descripcion_enf(reflujo, "Irritación por ácido.").
enf_sintoma(gripe, fiebre).
enf_sintoma(gripe, tos).
enf_sintoma(gripe, dolor_garganta).
enf_sintoma(reflujo, pirosis).
enf_sintoma(reflujo, regurgitacion).
enf_contra_medicamento(gripe, ibuprofeno).
enf_contra_medicamento(reflujo, aines).

medicamento(aines).
medicamento(ibuprofeno).
