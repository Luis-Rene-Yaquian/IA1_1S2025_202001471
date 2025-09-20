% ======= MediLogic KB (auto-generado) =======
% NO editar a mano; use /admin/kb

sintoma(cefalea).
sintoma(disnea).
sintoma(dolor_garganta).
sintoma(dolor_pecho).
sintoma(fiebre).
sintoma(nausea).
sintoma(pirosis).
sintoma(regurgitacion).
sintoma(tos).

enfermedad(asma, "Asma", respiratorio, cronico).
enfermedad(gripe, "Gripe", respiratorio, viral).
enfermedad(reflujo, "Enfermedad por reflujo gastroesofágico", digestivo, cronico).
descripcion_enf(asma, "Obstrucción reversible de la vía aérea.").
descripcion_enf(gripe, "Infección respiratoria alta.").
descripcion_enf(reflujo, "Irritación por ácido.").
enf_sintoma(asma, disnea).
enf_sintoma(asma, tos).
enf_sintoma(gripe, fiebre).
enf_sintoma(gripe, tos).
enf_sintoma(gripe, dolor_garganta).
enf_sintoma(reflujo, pirosis).
enf_sintoma(reflujo, regurgitacion).
enf_contra_medicamento(gripe, ibuprofeno).
enf_contra_medicamento(reflujo, aines).

medicamento(aines).
medicamento(ibuprofeno).
medicamento(omeprazol).
medicamento(paracetamol).
medicamento(salbutamol).
trata(aines, reflujo).
trata(ibuprofeno, gripe).
trata(omeprazol, reflujo).
trata(paracetamol, gripe).
trata(salbutamol, asma).
contraindicado(omeprazol, prolongacion_qt).
contraindicado(paracetamol, alergia_paracetamol).
